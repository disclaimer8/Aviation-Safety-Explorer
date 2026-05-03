# Architecture & Integration Guide (for Claude)

Hello Claude! This document provides a comprehensive technical overview of the **Aviation Safety Explorer** project. The user wants to integrate this subsystem into the `himaxym.com` platform. This guide will give you all the context you need regarding the database schema, scraping mechanisms, and API contracts.

## 1. System Overview

This system is a high-performance backend written in **Go 1.20+**. It operates as both a web server (exposing a RESTful JSON API) and an autonomous background worker (scraping data from multiple global aviation databases, deduplicating it, and geocoding textual locations into GPS coordinates).

### Core Responsibilities:
- **Data Aggregation**: Scrapes Aviation Safety Network (ASN), B3A (Bureau of Aircraft Accidents Archives), and Wikidata.
- **Deduplication**: Normalizes dates and fuzzy-matches aircraft models to prevent duplicate accident entries, merging URLs from different sources.
- **Geocoding**: A background worker converts textual locations to `lat`/`lon` using the OpenStreetMap Nominatim API.
- **API Delivery**: Serves fast, cached JSON endpoints for frontend visualization (tables, charts, maps).

## 2. Technology Stack

- **Language**: Go (Golang)
- **Database**: SQLite (`github.com/mattn/go-sqlite3` via CGO)
- **Web Framework**: Gin (`github.com/gin-gonic/gin`)
- **Scraping Engine**: Headless Chrome via `github.com/go-rod/rod` and `stealth` plugin (bypasses Cloudflare on ASN and B3A).
- **Deployment**: Multi-stage Dockerfile (Golang builder -> Debian slim with Chromium).

## 3. Database Schema

The system relies on a single SQLite file: `accidents.db`.

```sql
CREATE TABLE IF NOT EXISTS accidents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    normalized_date TEXT,   -- YYYY-MM-DD for strict sorting and deduplication
    date TEXT,              -- Original date string (e.g. "12 Jan 1980")
    aircraft_model TEXT,    -- e.g. "Cessna 208B Grand Caravan"
    operator TEXT,          -- Airline / Operator name
    fatalities TEXT,        -- String representation (e.g., "15+2", "Unknown")
    location TEXT,          -- Textual location (e.g., "near Juba")
    source_url TEXT,        -- Comma-separated list of URLs if merged from multiple sources
    lat REAL,               -- Latitude (populated by background geocoder, default NULL)
    lon REAL                -- Longitude (populated by background geocoder, default NULL)
);

-- Performance Indexes
CREATE INDEX IF NOT EXISTS idx_aircraft ON accidents(aircraft_model);
CREATE INDEX IF NOT EXISTS idx_operator ON accidents(operator);
CREATE INDEX IF NOT EXISTS idx_date ON accidents(date);
```

### Deduplication Logic
Before inserting, the system attempts to find an existing record where `normalized_date` matches exactly, and either the `aircraft_model` contains words from the new model or vice-versa. If a match is found, it appends the new `source_url` using `, ` and ignores the rest to preserve data integrity.

## 4. Background Workers

### Scrapers (Run via CLI flags)
- **ASN (`scraper_asn.go`)**: Launches a headless browser, waits 4 seconds to solve Cloudflare JS challenges, and extracts `table tbody tr` rows.
- **B3A (`scraper_b3a.go`)**: Navigates headless browser, extracts dynamically generated links like `/crash/crash-cessna...` and parses the aircraft model from the URL slug.
- **Wikidata (`scraper_wikidata.go`)**: Executes a single HTTP GET request to the Wikidata SPARQL endpoint (`wd:Q744913`) mapping `P8761` / `P289` properties.

### Geocoder (`geocoder.go`)
- Automatically starts when the web server runs.
- Executes `SELECT id, location FROM accidents WHERE lat IS NULL AND location != '' LIMIT 1`.
- Requests `https://nominatim.openstreetmap.org/search?q=...`
- Pauses for **1.5 seconds** between requests to strictly obey Nominatim's Acceptable Use Policy.
- Updates `lat` and `lon`. If not found, sets coordinates to `0.000001` to prevent infinite retry loops.

## 5. API Endpoints (Integration Points)

All endpoints return JSON and include HTTP caching headers (`Cache-Control: public, max-age=300`).

#### `GET /api/accidents?limit=100&offset=0`
Returns raw paginated accident data.
```json
{
  "limit": 100,
  "offset": 0,
  "data": [
    {
      "id": 1,
      "date": "10 Feb 2026",
      "aircraft_model": "Boeing 737",
      "operator": "Delta Airlines",
      "fatalities": "0",
      "location": "New York",
      "source_url": "https://...",
      "lat": 40.7128,
      "lon": -74.0060
    }
  ]
}
```

#### `GET /api/stats/aircrafts`
Returns the Top 10 aircraft models involved in accidents. The backend dynamically casts the textual `fatalities` column to `INTEGER` during the `SUM` SQL aggregation to ensure mathematical accuracy.
```json
[
  {
    "name": "Cessna 208B Grand Caravan",
    "count": 102,
    "fatalities": 103
  }
]
```

#### `GET /api/stats/operators`
Returns the Top 10 operators by accident count.
```json
[
  {
    "name": "Aeroflot",
    "count": 45,
    "fatalities": 300
  }
]
```

#### `GET /api/map_data`
A highly optimized, lightweight endpoint returning only non-null coordinates for rendering Map clusters (e.g., Leaflet.js).
```json
[
  {
    "id": 1,
    "model": "Boeing 737",
    "fatalities": "0",
    "lat": 40.7128,
    "lon": -74.0060
  }
]
```

## 6. How to Integrate into himaxym.com

**Option A (Microservice):**
Deploy this Go application using the provided `docker-compose.yml` alongside the `himaxym.com` stack. Route requests to `himaxym.com/api/aviation/*` to this container (port 8080) via an Nginx reverse proxy. You can use the frontend code (`static/index.html` with Chart.js and Leaflet) as an iframe or rewrite it in your frontend framework (React/Vue) by consuming the API directly.

**Option B (Data Extraction):**
If `himaxym.com` requires the raw data natively, you can run the Go scraper on a cron schedule locally, mount the `accidents.db` file, and read from it directly using your primary backend language, bypassing the Go Gin API entirely.
