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

	rows, err := db.Query("SELECT version, name FROM schema_versions ORDER BY version")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("schema_versions:")
	for rows.Next() {
		var v int
		var n string
		rows.Scan(&v, &n)
		fmt.Printf("  %03d: %s\n", v, n)
	}
}
