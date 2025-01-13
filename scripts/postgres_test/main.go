package main

import (
	"context"
	"log"

	"github.com/desain-gratis/common/repository/content"
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

	pgPostData := content.Data{
		Data: []byte(`{"phones":[{"type":"mobile","phone":"001001"},{"type":"fix","phone":"002002"}]}`),
	}

	_, err := pgDriver.Post(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1"}, pgPostData)
	if err != nil {
		log.Println("post error", err)
		return
	}

	log.Println("post done")

	resp, err := pgDriver.Get(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1"})
	if err != nil {
		log.Println("get error", err)
		return
	}

	log.Println("done select", resp)

	pgPutData := content.Data{
		Data: []byte(`{"updated_phones":[{"type":"mobile","phone":"001001"},{"type":"fix","phone":"002002"}]}`),
	}
	_, err = pgDriver.Put(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1"}, pgPutData)
	if err != nil {
		log.Println("put payload error", err)
		return
	}

	log.Println("done put payload")

	resp, err = pgDriver.Get(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1"})
	if err != nil {
		log.Println("get error", err)
		return
	}

	log.Println("done select payload", resp)

	_, err = pgDriver.Put(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1"}, content.Data{RefIDs: []string{"updated_ref_id_1_val_1"}})
	if err != nil {
		log.Println("put ref id error", err)
		return
	}

	log.Println("done put ref id")

	resp, err = pgDriver.Get(context.Background(), "user_id_val_1", "id_val_1", []string{"updated_ref_id_1_val_1"})
	if err != nil {
		log.Println("get ref id error", err)
		return
	}

	log.Println("done select ref id", resp)

	_, err = pgDriver.Delete(context.Background(), "user_id_val_1", "id_val_1", []string{"updated_ref_id_1_val_1"})
	if err != nil {
		log.Println("delete error", err)
	}

	log.Println("done delete")

	resp, err = pgDriver.Get(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1"})
	if err != nil {
		log.Println("get delete error", err)
		return
	}

	log.Println("done get delete", resp)
}

func doMultipleRefID(db *sqlx.DB) {
	pgDriver := postgres.New(db, "test_table_2")

	pgPostData := content.Data{
		Data: []byte(`{"phones":[{"type":"mobile","phone":"001001"},{"type":"fix","phone":"002002"}]}`),
	}

	_, err := pgDriver.Post(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1", "ref_id_1_val_2"}, pgPostData)
	if err != nil {
		log.Println("post error", err)
		return
	}

	log.Println("post done")

	resp, err := pgDriver.Get(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1", "ref_id_1_val_2"})
	if err != nil {
		log.Println("get error", err)
		return
	}

	log.Println("done select", resp)

	pgPutData := content.Data{
		Data: []byte(`{"updated_phones":[{"type":"mobile","phone":"001001"},{"type":"fix","phone":"002002"}]}`),
	}

	_, err = pgDriver.Put(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1", "ref_id_1_val_2"}, pgPutData)
	if err != nil {
		log.Println("put payload error", err)
		return
	}

	log.Println("done put payload")

	resp, err = pgDriver.Get(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1", "ref_id_1_val_2"})
	if err != nil {
		log.Println("get payload error", err)
		return
	}

	log.Println("done get payload", resp)

	_, err = pgDriver.Put(context.Background(), "user_id_val_1", "id_val_1", []string{"ref_id_1_val_1", "ref_id_1_val_2"}, content.Data{RefIDs: []string{"updated_ref_id_1_val_1", "ref_id_1_val_2"}})
	if err != nil {
		log.Println("put ref id error", err)
		return
	}

	log.Println("done put ref id")

	resp, err = pgDriver.Get(context.Background(), "user_id_val_1", "id_val_1", []string{"updated_ref_id_1_val_1", "ref_id_1_val_2"})
	if err != nil {
		log.Println("select ref id error", err)
		return
	}

	log.Println("done select ref id", resp)

	_, err = pgDriver.Delete(context.Background(), "user_id_val_1", "id_val_1", []string{"updated_ref_id_1_val_1", "ref_id_1_val_2"})
	if err != nil {
		log.Println("delete error", err)
	}

	log.Println("done delete")

	resp, err = pgDriver.Get(context.Background(), "user_id_val_1", "id_val_1", []string{"updated_ref_id_1_val_1", "ref_id_1_val_2"})
	if err != nil {
		log.Println("get delete error", err)
		return
	}

	log.Println("done get delete", resp)
}
