package main

import (
	"strconv"
	"database/sql"
	"github.com/go-sql-driver/mysql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"github.com/PuerkitoBio/goquery"
)



func main() {

	// 1. Scrape data from wiki 
	wikiURL := "https://wikiwiki.jp/nenaiko/モンスター"
	responseDoc, err := fetchWiki(wikiURL)
	if err != nil {
		log.Fatalf("API fetch failed! : %v", err)
	}
	log.Print("Successfully fetched wiki document!")

	// 2. Connect to MySQL
	db, err := connectDB()
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	// 3. Parse wiki for info
	monsterNames := extractMonsterNames(responseDoc)

	// 4. Update MySQL records with English names
	if err := updateDb(db, monsterNames); err != nil {
		log.Fatalf("Database import failed: %v", err)
	}
	log.Println("Import completed successfully.")
}

func fetchWiki(url string) (*goquery.Document, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("User-Agent", "MH-API-NameImporter/1.0")

	client := &http.Client{Timeout: 15 * time.Second}

	resp, err := client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	//body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	//log.Printf("Wiki fetch completed with length %d bytes and status %s.", len(body), resp.Status)

	return goquery.NewDocumentFromReader(resp.Body)
}

func extractMonsterNames(doc *goquery.Document) map[string]string {
	monsters := make(map[string]string)

	doc.Find("table").Each(func(ii int, table *goquery.Selection) {

		var jpIndex, enIndex = -1, -1

		// Identigy relevant columns
		table.Find("tr").First().Find("th").Each(func(i int, cell *goquery.Selection) {
			header := strings.TrimSpace(cell.Text())

			if header == "モンスター名" {
				jpIndex = i
			}

			if header == "英名" {
				enIndex = i
			}

		})

		if jpIndex == -1 || enIndex == -1 {
			return // Skip if not found
		}
		var span int = 0
		table.Find("tr").Each(func(j int, row *goquery.Selection) {
			cells := row.Find("td")
			rowspan, _ := (cells.Eq(jpIndex+1)).Attr("rowspan")	// Check if a row starts a merged cell column
			span_current, _  := strconv.Atoi(rowspan)
			if span_current > 0 {
				span = span_current
			}

			log.Printf("|---- Parsing row %d of table %d (rowspan=%d)... | ", j, ii, span)

			if cells.Length() <= jpIndex || cells.Length() <= enIndex {
				log.Print("Skipping due to row underflow...")
				return // Skip if not enough columns
			}

			jpCell := cells.Eq(jpIndex)
			enCell := cells.Eq(enIndex)
			if (span > 0 && rowspan != "") {
				span--
			} else if (span > 0 && rowspan == "") {
				span--
				enCell = cells.Eq(enIndex - 1)
			}
			
			// Only process unambiguous single-name rows
			names := jpCell.Find("a")
			if names.Length() < 1 {
				log.Print("Skipping due to <a> tag underflow...")
				return
			}

			japanese := strings.TrimSpace(names.First().Text())
			english := strings.TrimSpace(enCell.Text())
			
			log.Printf("JP: %s, EN: %s", japanese, english)

			if japanese == "" || english == "" || english == "-" {
				log.Print("One of the names were blank! Not pushing to DB...")
				return
			}
			log.Println("")

			monsters[japanese] = english
		})

	})

	return monsters
}

func connectDB() (*sql.DB, error) {
	// Configure DSN safely using mysql.Config
	cfg := mysql.Config{
		User:                 "root",
		Passwd:               "7a805e2b42ea245a287f3f2a40e41b5e05c8e521a307cae8",
		Net:                  "tcp",
		Addr:                 "127.0.0.1:3306",
		DBName:               "mh-api",
		ParseTime:            true,
		AllowNativePasswords: true,
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}

	// Configure connection pool settings
	db.SetConnMaxLifetime(3 * time.Minute)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	if err := db.Ping(); err != nil {
		return nil, err
	}

	log.Print("Connected to MySQL database")
	return db, nil
}

func updateDb(db *sql.DB, names map[string]string) error {
	for japanese, english := range names {

		_, err := db.Exec("UPDATE monster SET name_en = ? WHERE name = ?", english, japanese)

		if err != nil {
			return fmt.Errorf("updating monster %q: %w", japanese, err)
		}
	}
	return nil
}
