package main

import (
	"strconv"
	"encoding/json"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"github.com/PuerkitoBio/goquery"
	"github.com/go-sql-driver/mysql"
)

type Response struct {
	Monsters []Monster `json:"monsters"`
}

type Monster struct {
	Name  string `json:"name_en"`
	ID    string `json:"monster_id"`
}

func main() {

	// 1. Scrape data from wiki 
	const wikiURL = "https://thehunterpedia.com/monsters/"		// JP Wiki: "https://wikiwiki.jp/nenaiko/モンスター"
	responseDoc, err := fetchEnApi()					// fetchJpWiki(wikiURL)
	if err != nil {
		log.Fatalf("Wiki fetch failed! : %v", err)
	}
	log.Print("Successfully fetched wiki document!")

	// 2. Connect to MySQL
	db, err := connectDB()
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	// 3a. Parse JP wiki for info
	// monsterNames := extractMonsterNames(responseDoc)

	// 3b. Parse EN wiki for info, returns map[string][]int
	monsterWeaknesses := extractMonsterWeaknesses(responseDoc, wikiURL)

	/* 4a. Update MySQL monster table with English names
	if err := updateMonsterDb(db, monsterNames); err != nil {
		log.Fatalf("Database import failed: %v", err)
	}
	log.Println("Import completed successfully.") */

	// 4b. Update MySQL weakness table with hitzone info
	if err := updateWeaknessDb(db, monsterWeaknesses); err != nil {
		log.Fatalf("Database import failed: %v", err)
	}
	log.Println("Import completed successfully.")
}

func fetchJpWiki(url string) (*goquery.Document, error) {
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

	log.Printf("Wiki fetch completed with length %d and status %s.", resp.ContentLength, resp.Status)

	return goquery.NewDocumentFromReader(resp.Body)
}

func fetchEnApi() (*Response, error) {

	// Get list of monsters from the API
	reqApi, err := http.NewRequest("GET", "https://141.148.71.142/v1/monsters?limit=333", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	respApi, err := client.Do(reqApi)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer respApi.Body.Close()

	if respApi.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %s", respApi.Status)
	}

	var response Response
	if err := json.NewDecoder(respApi.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}

	return &response, nil
}

func fetchWithDefer(c *http.Client, url string) (*goquery.Document, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("User-Agent", "MH-API-NameImporter/1.0")

	resp, err := c.Do(req)

	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	return goquery.NewDocumentFromReader(resp.Body)
}

func extractMonsterWeaknesses(response *Response, url string) map[string][]int {

	// Assumed that all weaknesses are in the same order as database
	monster_arr := make(map[string][]int)

	// Begin monster info requisition loop
	client := &http.Client{Timeout: 15 * time.Second}
	for _, monster := range response.Monsters {
		// Fetch and parse each monster's hitzones
		if monster.Name == ""{
			continue
		}

		url_monster_name := strings.ReplaceAll(strings.TrimSpace(strings.ToLower(monster.Name)), " ", "-")
		full_url := fmt.Sprintf("%s%s", url, url_monster_name)
		resp, err := fetchWithDefer(client, full_url)

		if err != nil {
			log.Printf("Failed to fetch info for %q: %v", monster.Name, err)
			continue
		}

		weaknesses := parseWeaknessHtml(resp, monster.ID)

		monster_arr[monster.Name] = weaknesses

	}

	return monster_arr
}

func parseWeaknessHtml(doc *goquery.Document, monster_id string) []int {
	var weaknesses []int

	doc.Find("table").First().Find("tbody").First().Find("tr").First().Find("td").Each(func(i int, cell *goquery.Selection) {
		var mult string = ""
		if i == 0 {
			mult = monster_id	// Skip game name cell, use column for monster ID instead
		} else if (cell.Text() == "-" || cell.Length() < 1) {
			mult = "0"
		} else {
			mult = strings.TrimSpace(cell.Text())
		}

		multInt, _ := strconv.Atoi(mult)
		weaknesses = append(weaknesses, multInt)
	})

	return weaknesses
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
                        rowspan, _ := (cells.Eq(jpIndex+1)).Attr("rowspan")     // Check if a row starts a merged cell column
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

func updateMonsterDb(db *sql.DB, names map[string]string) error {
	for japanese, english := range names {
		fmt.Printf("%s -> %s...  | ", japanese, english)

		_, err := db.Exec("UPDATE monster SET name_en = ? WHERE name = ?", english, japanese)

		if err != nil {
			return fmt.Errorf("updating monster %q: %w", japanese, err)
		} else {
			fmt.Print("Success.\n")
		}
	}
	return nil
}

func updateWeaknessDb(db *sql.DB, weaknesses map[string][]int) error {
	for monster, values := range weaknesses {
		fmt.Printf("Adding weaknesses for %s (id: %d)...  | ", monster, values[0])

		_, err := db.Exec(`
		INSERT INTO weakness (monster_id, aprt_id, slashing, blow, bullet, fire, water, lightning, ice, dragon)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			monster_id = VALUES(monster_id),
			aprt_id = VALUES(aprt_id),
			slashing = VALUES(slashing),
			blow = VALUES(blow),
			bullet = VALUES(bullet),
			fire = VALUES(fire),
			water = VALUES(water),
			lightning = VALUES(lightning),
			ice = VALUES(ice),
			dragon = VALUES(dragon)
		`, values[0], values[0], values[1], values[2], values[3], values[4], values[5], values[6], values[7], values[8])

		if err != nil {
			return fmt.Errorf("updating monster %q: %w", monster, err)
		} else {
			fmt.Print("Success.\n")
		}
	}

	return nil
}
