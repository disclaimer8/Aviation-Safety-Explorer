package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

// ScrapeB3A begins the scraping process for the Bureau of Aircraft Accidents Archives.
func ScrapeB3A(db *sql.DB, startYear, endYear int) {
	c := colly.NewCollector(
		colly.AllowedDomains("www.baaa-acro.com", "baaa-acro.com"),
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"),
	)

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*baaa-acro.com*",
		Parallelism: 1,
		RandomDelay: 2 * time.Second,
	})

	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting", r.URL.String())
	})

	// Assuming a standard table structure on B3A archives.
	// B3A usually has tables with Date, Aircraft, Operator, Location.
	c.OnHTML("table tbody tr", func(e *colly.HTMLElement) {
		cells := e.ChildTexts("td")
		if len(cells) < 4 {
			return
		}

		date := strings.TrimSpace(cells[0])
		model := strings.TrimSpace(cells[1])
		operator := strings.TrimSpace(cells[2])
		
		// B3A sometimes mixes location or fatalities in later columns.
		// We'll extract what we confidently can.
		location := ""
		if len(cells) > 3 {
			location = strings.TrimSpace(cells[3])
		}

		fatalities := ""
		if len(cells) > 4 {
			fatalities = strings.TrimSpace(cells[4])
		}

		sourceURL := e.Request.AbsoluteURL(e.ChildAttr("td a", "href"))
		if sourceURL == "" {
			sourceURL = e.Request.URL.String() + "#" + date
		}

		if date == "" || model == "" {
			return
		}

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
			log.Printf("Error saving B3A accident: %v\n", err)
		}
	})

	c.OnError(func(r *colly.Response, err error) {
		log.Println("Request URL:", r.Request.URL, "failed with error:", err)
	})

	for year := startYear; year <= endYear; year++ {
		// B3A year archive URL structure (approximate)
		url := fmt.Sprintf("https://www.baaa-acro.com/crash-archives?year=%d", year)
		err := c.Visit(url)
		if err != nil {
			log.Printf("Failed to visit B3A year %d: %v\n", year, err)
		}
	}
	
	c.Wait()
}
