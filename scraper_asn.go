package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

// ScrapeASN begins the scraping process for ASN.
func ScrapeASN(db *sql.DB, startYear, endYear int) {
	c := colly.NewCollector(
		colly.AllowedDomains("aviation-safety.net"),
		// User-agent helps to avoid being blocked immediately
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"),
	)

	// Add random delay to prevent IP ban
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*aviation-safety.net*",
		Parallelism: 2,
		RandomDelay: 2 * time.Second,
	})

	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting", r.URL.String())
	})

	// ASN database table rows
	// The table usually has the class 'list' or we can target standard rows.
	// Columns are typically: Date, Type, Registration, Operator, Fat., Location, Cat
	c.OnHTML("table tbody tr", func(e *colly.HTMLElement) {
		// Skip header rows if any
		if e.DOM.HasClass("header") || e.ChildText("th") != "" {
			return
		}

		cells := e.ChildTexts("td")
		if len(cells) < 5 {
			return // Not a valid data row
		}

		date := strings.TrimSpace(cells[0])
		model := strings.TrimSpace(cells[1])
		
		operator := ""
		if len(cells) > 3 {
			operator = strings.TrimSpace(cells[3])
		}
		
		fatalities := ""
		if len(cells) > 4 {
			fatalities = strings.TrimSpace(cells[4])
		}
		
		location := ""
		if len(cells) > 5 {
			location = strings.TrimSpace(cells[5])
		}

		// Extract link to the detail page
		relativeURL := e.ChildAttr("td:nth-child(1) a", "href")
		sourceURL := ""
		if relativeURL != "" {
			sourceURL = e.Request.AbsoluteURL(relativeURL)
		} else {
			// If no link is available, use the current page URL + a unique identifier just in case
			sourceURL = e.Request.URL.String() + "#" + date + "-" + model
		}

		// Only save if it has a date and a model
		if date != "" && model != "" {
			accident := Accident{
				Date:          date,
				AircraftModel: model,
				Operator:      operator,
				Fatalities:    fatalities,
				Location:      location,
				SourceURL:     sourceURL,
			}
			err := InsertAccident(db, accident)
			if err != nil {
				log.Printf("Error saving accident: %v\n", err)
			}
		}
	})

	// Handle pagination (if they have Next links on the year page)
	c.OnHTML("div.pagenumbers a", func(e *colly.HTMLElement) {
		nextPage := e.Attr("href")
		if strings.Contains(strings.ToLower(e.Text), "next") || strings.Contains(e.Text, ">") {
			c.Visit(e.Request.AbsoluteURL(nextPage))
		}
	})

	c.OnError(func(r *colly.Response, err error) {
		log.Println("Request URL:", r.Request.URL, "failed with response:", r.StatusCode, "\nError:", err)
	})

	// Loop through years
	for year := startYear; year <= endYear; year++ {
		url := fmt.Sprintf("https://aviation-safety.net/database/year/%d/1", year)
		err := c.Visit(url)
		if err != nil {
			log.Printf("Failed to visit year %d: %v\n", year, err)
		}
	}
	
	// Wait for all goroutines to finish
	c.Wait()
}
