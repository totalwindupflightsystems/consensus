package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := os.Args[1]
	db, err := sql.Open("sqlite", dbPath+"?_time_format=sqlite")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT version, name FROM schema_versions ORDER BY version")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("=== schema_versions ===")
	applied := make(map[int]string)
	for rows.Next() {
		var v int
		var n string
		rows.Scan(&v, &n)
		applied[v] = n
		fmt.Printf("  %03d: %s\n", v, n)
	}

	fmt.Println("\n=== Embedded migration files ===")
	migDir := "/home/kara/conscientiousness/internal/migrate/migrations"
	entries, _ := os.ReadDir(migDir)
	for _, e := range entries {
		name := e.Name()
		// Extract version number
		var version int
		if _, err := fmt.Sscanf(name, "%d", &version); err == nil {
			_, found := applied[version]
			status := "FOUND" 
			if !found {
				status = "MISSING"
			} else {
				// Check if name matches
				if !strings.Contains(applied[version], filepath.Base(name)) {
					status = "NAME_MISMATCH"
				}
			}
			fmt.Printf("  %s → %s (version %d)\n", name, status, version)
		}
	}

	fmt.Println("\n=== SQLite master (user tables) ===")
	trows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		log.Fatal(err)
	}
	count := 0
	for trows.Next() {
		var n string
		trows.Scan(&n)
		count++
	}
	fmt.Printf("  Total user tables: %d\n", count)
}
