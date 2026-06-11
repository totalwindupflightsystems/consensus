package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := os.Args[1]
	db, err := sql.Open("sqlite", dbPath+"?_time_format=sqlite")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var count, maxVer int
	var maxName string
	err = db.QueryRow("SELECT COUNT(*), COALESCE(MAX(version),0), COALESCE(MAX(name),'') FROM schema_versions").Scan(&count, &maxVer, &maxName)
	if err != nil {
		log.Fatalf("schema_versions query: %v", err)
	}
	fmt.Printf("schema_versions: %d rows, max_version=%d, max_name=%s\n", count, maxVer, maxName)

	var tableCount int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&tableCount)
	if err != nil {
		log.Fatalf("table count: %v", err)
	}
	fmt.Printf("User tables: %d\n", tableCount)
}
