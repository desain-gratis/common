package main

import (
	"context"
	"log"

	"github.com/desain-gratis/common/repository/content/postgres"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var scheme_1 = "CREATE TABLE IF NOT EXISTS test_table_1 (user_id VARCHAR NOT NULL, id VARCHAR NOT NULL, ref_id_1 VARCHAR, payload JSONB NOT NULL, PRIMARY KEY (user_id, id, ref_id_1));"
var scheme_2 = "CREATE TABLE IF NOT EXISTS test_table_2 (user_id VARCHAR NOT NULL, id VARCHAR NOT NULL, ref_id_1 VARCHAR NOT NULL, ref_id_2 VARCHAR NOT NULL, payload JSONB NOT NULL, PRIMARY KEY (user_id, id, ref_id_1, ref_id_2));"

func main() {
	db, err := sqlx.Connect("postgres", "user=bytedance dbname=test_db sslmode=disable password=root host=localhost")
	if err != nil {
		log.Fatalln(err)
	}

	defer db.Close()

	// Test the connection to the database
	if err := db.Ping(); err != nil {
		log.Fatal(err)
	} else {
		log.Println("Successfully Connected")
	}

	_, err = db.Exec(scheme_1)
	if err != nil {
		log.Println("error", err)
		return
	}

	_, err = db.Exec(scheme_2)
	if err != nil {
		log.Println("error", err)
		return
	}

	doSingleRefID(db)
	doMultipleRefID(db)
}

func doSingleRefID(db *sqlx.DB) {
	pgDriver := postgres.New(db, "test_table_1")

	err := pgDriver.Insert(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1"}, `{"phones":[{"type":"mobile","phone":"001001"},{"type":"fix","phone":"002002"}]}`)
	if err != nil {
		log.Println("insert error", err)
		return
	}

	log.Println("insert done")

	resp, err := pgDriver.Select(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1"})
	if err != nil {
		log.Println("select error", err)
		return
	}

	log.Println("done select", resp)

	err = pgDriver.Update(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1"}, postgres.UpsertData{PayloadJSON: `{"updated_phones":[{"type":"mobile","phone":"001001"},{"type":"fix","phone":"002002"}]}`})
	if err != nil {
		log.Println("update payload error", err)
		return
	}

	log.Println("done update payload")

	resp, err = pgDriver.Select(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1"})
	if err != nil {
		log.Println("select payload error", err)
		return
	}

	log.Println("done select payload", resp)

	err = pgDriver.Update(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1"}, postgres.UpsertData{RefIDs: []string{"updated_ref_id_1_val_1"}})
	if err != nil {
		log.Println("update ref id error", err)
		return
	}

	log.Println("done update ref id")

	resp, err = pgDriver.Select(context.Background(), "user_id_val_1", "id_val_1", []string{"updated_ref_id_1_val_1"})
	if err != nil {
		log.Println("select ref id error", err)
		return
	}

	log.Println("done select ref id", resp)

	err = pgDriver.Delete(context.Background(), "user_id_val_1", "id_val_1", []string{"updated_ref_id_1_val_1"})
	if err != nil {
		log.Println("delete error", err)
	}

	log.Println("done delete")

	resp, err = pgDriver.Select(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1"})
	if err != nil {
		log.Println("select delete error", err)
		return
	}

	log.Println("done select delete", resp)
}

func doMultipleRefID(db *sqlx.DB) {
	pgDriver := postgres.New(db, "test_table_2", 100)

	err := pgDriver.Insert(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1", "ref_id_1_val_2"}, `{"phones":[{"type":"mobile","phone":"001001"},{"type":"fix","phone":"002002"}]}`)
	if err != nil {
		log.Println("insert error", err)
		return
	}

	log.Println("insert done")

	resp, err := pgDriver.Select(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1", "ref_id_1_val_2"})
	if err != nil {
		log.Println("select error", err)
		return
	}

	log.Println("done select", resp)

	err = pgDriver.Update(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1", "ref_id_1_val_2"}, postgres.UpsertData{PayloadJSON: `{"updated_phones":[{"type":"mobile","phone":"001001"},{"type":"fix","phone":"002002"}]}`})
	if err != nil {
		log.Println("update payload error", err)
		return
	}

	log.Println("done update payload")

	resp, err = pgDriver.Select(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1", "ref_id_1_val_2"})
	if err != nil {
		log.Println("select payload error", err)
		return
	}

	log.Println("done select payload", resp)

	err = pgDriver.Update(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1", "ref_id_1_val_2"}, postgres.UpsertData{RefIDs: []string{"updated_ref_id_1_val_1", "ref_id_1_val_2"}})
	if err != nil {
		log.Println("update ref id error", err)
		return
	}

	log.Println("done update ref id")

	resp, err = pgDriver.Select(context.Background(), "user_id_val_1", "id_val_1", []string{"updated_ref_id_1_val_1", "ref_id_1_val_2"})
	if err != nil {
		log.Println("select ref id error", err)
		return
	}

	log.Println("done select ref id", resp)

	err = pgDriver.Delete(context.Background(), "user_id_val_1", "id_val_1", []string{"updated_ref_id_1_val_1", "ref_id_1_val_2"})
	if err != nil {
		log.Println("delete error", err)
	}

	log.Println("done delete")

	resp, err = pgDriver.Select(context.Background(), "user_id_val_1", "id_val_1", []string{"updated_ref_id_1_val_1", "ref_id_1_val_2"})
	if err != nil {
		log.Println("select delete error", err)
		return
	}

	log.Println("done select delete", resp)
}
