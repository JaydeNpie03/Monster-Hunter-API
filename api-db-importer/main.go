package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
	"strings"
	"github.com/go-sql-driver/mysql"
)

// Each struct represents the structure of each nested level of the API's JSON response
// =============== BEGIN API STRUCTS ===============
type Response struct {
	Total int `json:"total"`
	Monsters []Monster `json:"monsters"`
}

type Monster struct {
	ID    string    `json:"monster_id"`
	Name  string `json:"name"`
	Element string `json:"element"`
	Alias string `json:"another_name"`
	Category string `json:"category"`
	BGM []Music `json:"bgm"`
}

type Music struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
// ================ END API STRUCTS ================

// =========== BEGIN STATIC JSON STRUCTS ===========
type ResponseStatic struct {
	Monsters []MonsterStatic `json:"monsters"`
}

type MonsterStatic struct {
	Name    string `json:"name"`
	GameInfo []GameInfo `json:"games"`
	Category string `json:"type"`
	Elements []string `json:"elements"`
	Ailments []string `json:"ailments"`
	Weaknesses []string `json:"weakness"`
}

type GameInfo struct {
	Game string `json:"game"`
	ImageIcon  string `json:"image"`
	Description string `json:"info"`
}
// ============ END STATIC JSON STRUCTS ============



// ================= Functions =================

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	/* 1a. Fetch data from the external API
	apiURL := "https://api.mh-api.com/v1/monsters?limit=333"
	response, err := fetchApiInfo(ctx, apiURL)
	if err != nil {
			log.Fatalf("API fetch failed: %v", err)
	}
	log.Printf("Fetched %d record(s) from API", response.Total)
	*/

	// 1b. Fetch data from a static JSON file
	staticFilePath := "https://raw.githubusercontent.com/CrimsonNynja/monster-hunter-DB/refs/heads/master/monsters.json"
	staticResponse, err := fetchStaticInfo(staticFilePath)
	if err != nil {
		log.Fatalf("Static file fetch failed: %v", err)
	}
	log.Printf("Fetched %d record(s) from static file", len(staticResponse.Monsters))

	// 2. Connect to MySQL
	db, err := connectDB()
	if err != nil {
			log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	/* 3a. Import API records into MySQL
	if err := saveApiInfo(ctx, db, response); err != nil {
			log.Fatalf("Database import failed: %v", err)
	}
	log.Println("Import completed successfully.")
	*/

	// 3b. Import static JSON records into MySQL
	if err := saveStaticInfo(ctx, db, staticResponse); err != nil {
		log.Fatalf("Database import failed: %v", err)
	}
	log.Println("Import completed successfully.")
}

func fetchApiInfo(ctx context.Context, url string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

	var response Response
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}

	return &response, nil
}

func fetchStaticInfo(url string) (*ResponseStatic, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching static info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	var response ResponseStatic
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}

	return &response, nil
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

func saveApiInfo(ctx context.Context, db *sql.DB, response *Response) error {
	// Begin a transaction so all inserts succeed or fail together
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare an upsert statement for the monster table using '?' placeholders
	monsterQuery := `
		INSERT INTO monster (monster_id, name, another_name, element)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			monster_id = VALUES(monster_id),
			name = VALUES(name),
			another_name = VALUES(another_name),
			element = VALUES(element)`

	monsterStmt, err := tx.PrepareContext(ctx, monsterQuery)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer monsterStmt.Close()


	// Prepare an upsert statement for the music table using '?' placeholders
	musicQuery := `
		INSERT INTO music (music_id, monster_id, name, url)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			music_id = VALUES(monster_id),
			monster_id = VALUES(monster_id),
			name = VALUES(name),
			url = VALUES(url)`

	musicStmt, err := tx.PrepareContext(ctx, musicQuery)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer musicStmt.Close()

	// Execute the upsert for each monster into each relevant table
	for _, m := range response.Monsters {
		// Update monsters table
		log.Printf("Now adding %s and its music...", m.Name)

		if _, err := monsterStmt.ExecContext(ctx, m.ID, m.Name, m.Alias, m.Element); err != nil {
			return fmt.Errorf("inserting monster %d: %w", m.ID, err)
		}

		// Update music table
		if len(m.BGM) > 0 {
			if _, err := musicStmt.ExecContext(ctx, m.ID, m.ID, m.BGM[0].Name, strings.Split(m.BGM[0].URL, "v=")[1]); err != nil {
				return fmt.Errorf("inserting monster %d: %w", m.ID, err)
			}
		} else {
			if _, err := musicStmt.ExecContext(ctx, m.ID, m.ID, "NULL", "NULL"); err != nil {
				return fmt.Errorf("inserting monster %d: %w", m.ID, err)
			}
		}

		log.Printf("Successfully added.")
	}

	return tx.Commit()
}

func saveStaticInfo(ctx context.Context, db *sql.DB, response *ResponseStatic) error {
	// Begin a transaction so all inserts succeed or fail together
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare an upsert statement for the monster table using '?' placeholders
	monsterQuery := `
		UPDATE monster SET description = ? WHERE name_en = ?`

	monsterStmt, err := tx.PrepareContext(ctx, monsterQuery)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer monsterStmt.Close()

	// Execute the upsert for each monster
	for _, m := range response.Monsters {
		// Update monsters table
		log.Printf("Now adding %s's description... ", m.Name)

		// Get description from latest game appearance
		index := 0 
		if (len(m.GameInfo) > 0) {
			index = len(m.GameInfo) - 1
		}

		if _, err := monsterStmt.ExecContext(ctx, m.GameInfo[index].Description, m.Name); err != nil {
			return fmt.Errorf("inserting monster %s: %w", m.Name, err)
		}

		log.Printf("Successfully added.")
	}

	return tx.Commit()
}
