package main

import (
	"encoding/json"
	"database/sql"
	"fmt"
	"log"
	"io"
	"os"
	"net/http"
	"time"
	"github.com/PuerkitoBio/goquery"
	"github.com/go-sql-driver/mysql"
)

type Response struct {
	Monsters []Monster `json:"monsters"`
}

type Monster struct {
	NameEn  string `json:"name_en"`
	NameJp string `json:name`
	ID    string `json:"monster_id"`
}

func main() {

	// 1. Scrape data from wiki 
	const wikiURL = "https://monsterhunterwiki.org/wiki/Monster_List"
	// responseDoc, err := fetchEnApi()
	//if err != nil {
	//	log.Fatalf("API fetch failed! : %v", err)
	//}
	//log.Print("Successfully fetched API info!")

	// 2. Connect to MySQL
	db, err := connectDB()
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	// 3a. Fetch wiki info
	wikiInfo, err := fetchWikiPage(wikiURL)
	if err != nil {
		log.Fatalf("Wiki fetch failed! : %v", err)
	}
	log.Print("Successfully fetched Wiki info!")

	// 3b. Parse wiki info
	monsterInfo := extractMonsterInfo(wikiInfo)
	log.Printf("Monster info parsed from wiki: %v\n\n", monsterInfo)

	// 4. Update MySQL monster table with English names if not present and download monster icons
	if err := updateMonsterDb(db, monsterInfo); err != nil {
		log.Fatalf("Database import failed: %v", err)
	}
	log.Println("Import completed successfully.")
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

func fetchWikiPage(url string) (*goquery.Document, error) {
	// Get list of monsters from the API
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}
	document, err := goquery.NewDocumentFromReader(resp.Body)

	return document, err
}

func extractMonsterInfo(doc *goquery.Document)  map[string]map[string]string {
	monsters := make(map[string]map[string]string)

	doc.Find(".mlistcol .left p").Each(func(_ int, data *goquery.Selection) {
		enName := data.Find("a").Text()
		jpName := data.Find("span").Eq(1).Text()
		icon_link, _ := data.Find("span").Eq(0).Find("a").Eq(1).Attr("href")

		innerMonster := make(map[string]string)
		innerMonster[enName] = icon_link
		monsters[jpName] = innerMonster
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

func updateMonsterDb(db *sql.DB, info map[string]map[string]string) error {
	for japanese, english := range info {
		for en_name, icon := range english {
			fmt.Printf("%s -> %s...  | ", japanese, en_name)

			result, err := db.Exec("UPDATE monster SET name_en = ? WHERE (name = ? AND name_en NOT LIKE '%[^a-zA-Z ]%')", en_name, japanese)
			if err != nil {
				return fmt.Errorf("updating monster %q: %w", japanese, err)
			}

			rows, err := result.RowsAffected()
			if rows > 0 {
				fmt.Printf("%s mapping to %s.\n", japanese, en_name)
			}

			if err != nil {
				return fmt.Errorf("updating monster %q: %w", japanese, err)
			}

			var monster_id string
			err = db.QueryRow("SELECT monster_id FROM monster WHERE name_en = ?", en_name).Scan(&monster_id)
			if err != nil {
				continue
				fmt.Errorf("retrieving monster id %q", en_name)
			}

			fmt.Printf("Downloading icon for %s...\n", en_name)
			err = downloadIconImage(icon, monster_id)

			if err != nil {
				return fmt.Errorf("downloading monster icon %q: %w", japanese, err)
			}
		}
	}
	
	return nil
}

func downloadIconImage(url string, id string) error {
    resp, err := http.Get(url)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("download failed: %s", resp.Status)
    }
    _, err = os.Stat(fmt.Sprintf("../data/monster/icons/%s_icon.png", id))
    if os.IsNotExist(err) {
        file, err := os.Create(fmt.Sprintf("../data/monster/icons/%s_icon.png", id))
    	if err != nil{
		return err
	}
        defer file.Close()
        _, err = io.Copy(file, resp.Body)
	return err

    }

    return err
}
