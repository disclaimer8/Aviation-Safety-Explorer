package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
)

// ScrapeASN begins the scraping process for ASN using a headless browser.
func ScrapeASN(db *sql.DB, startYear, endYear int) {
	fmt.Println("Initializing Headless Browser for ASN...")
	
	// Launch browser with stealth settings
	path, _ := launcher.LookPath()
	u := launcher.New().Bin(path).Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	for year := startYear; year <= endYear; year++ {
		url := fmt.Sprintf("https://aviation-safety.net/database/year/%d/1", year)
		fmt.Printf("Visiting ASN (Headless): %s\n", url)
		
		page := stealth.MustPage(browser)
		
		err := page.Navigate(url)
		if err != nil {
			log.Printf("Failed to navigate ASN %d: %v\n", year, err)
			page.MustClose()
			continue
		}

		page.MustWaitLoad()
		
		// Cloudflare might show a challenge. We wait a few seconds.
		time.Sleep(4 * time.Second)

		// Try to find the table
		rows, err := page.Elements("table tbody tr")
		if err != nil {
			log.Printf("Could not find table for year %d. CF blocked or no data.\n", year)
			page.MustClose()
			continue
		}

		for _, row := range rows {
			class, err := row.Attribute("class")
			if err == nil && class != nil && *class == "header" {
				continue
			}

			cells, err := row.Elements("td")
			if err != nil || len(cells) < 5 {
				continue
			}

			date := strings.TrimSpace(cells[0].MustText())
			model := strings.TrimSpace(cells[1].MustText())
			
			operator := ""
			if len(cells) > 3 {
				operator = strings.TrimSpace(cells[3].MustText())
			}
			
			fatalities := ""
			if len(cells) > 4 {
				fatalities = strings.TrimSpace(cells[4].MustText())
			}
			
			location := ""
			if len(cells) > 5 {
				location = strings.TrimSpace(cells[5].MustText())
			}

			sourceURL := ""
			link, err := cells[0].Element("a")
			if err == nil {
				href, _ := link.Property("href")
				if !href.Nil() {
					sourceURL = href.String()
				}
			}

			if sourceURL == "" {
				sourceURL = url + "#" + date + "-" + model
			}

			if date != "" && model != "" {
				accident := Accident{
					Date:          date,
					AircraftModel: model,
					Operator:      operator,
					Fatalities:    fatalities,
					Location:      location,
					SourceURL:     sourceURL,
				}
				InsertAccident(db, accident)
			}
		}

		page.MustClose()
		time.Sleep(3 * time.Second) // respectful delay
	}
}
