// [RUN] go run .
package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"strconv"

	"github.com/beego/beego/v2/server/web"
	_ "github.com/mattn/go-sqlite3"
)

type Option struct {
	Id      int64  `json:"id,omitempty"`
	Body    string `json:"body"`
	Correct bool   `json:"correct"`
}

type Question struct {
	Id      int64    `json:"id,omitempty"`
	Body    string   `json:"body"`
	Options []Option `json:"options"`
}

type AddOption struct {
	Body    string `json:"body"`
	Correct bool   `json:"correct"`
}

type AddQuestion struct {
	Body    string      `json:"body"`
	Options []AddOption `json:"options"`
}

func exitOnError(err error, additionalInfo string, exitCode int) {
	if err != nil {
		log.Printf("%q: %s\n", err, additionalInfo)
		os.Exit(1)
	}
}

func initDb(db *sql.DB) error {
	sqlStmt := `
        CREATE TABLE "options" (
            "id"	INTEGER NOT NULL UNIQUE,
            "body"	TEXT NOT NULL,
            "correct"	INTEGER NOT NULL,
            PRIMARY KEY("id" AUTOINCREMENT)
        )
    `
	_, err := db.Exec(sqlStmt)
	if err != nil {
		return err
	}

	sqlStmt = `
        CREATE TABLE "question_bodies" (
            "id"	INTEGER NOT NULL UNIQUE,
            "body"	TEXT NOT NULL,
            PRIMARY KEY("id")
        )
    `
	_, err = db.Exec(sqlStmt)
	if err != nil {
		return err
	}

	sqlStmt = `
        CREATE TABLE "questions" (
            "question_id"	INTEGER NOT NULL,
            "option_id"	INTEGER NOT NULL,
            "option_order"	INTEGER,
            FOREIGN KEY("question_id") REFERENCES "question_bodies"("id"),
            FOREIGN KEY("option_id") REFERENCES "options"("id"),
            UNIQUE("option_id","question_id","option_order"),
            PRIMARY KEY("question_id","option_id")
        )
    `
	_, err = db.Exec(sqlStmt)
	return err
}

func addQuestionOptionRelations(tx *sql.Tx, questionId int64, optionIds []int64) error {
	for index, optionId := range optionIds {
		stmt, err := tx.Prepare("INSERT INTO questions(\"question_id\", \"option_id\", \"option_order\") VALUES(?, ?, ?)")
		if err != nil {
			return err
		}
		defer stmt.Close()

		x, err := stmt.Exec(questionId, optionId, index)
		if err != nil {
			return err
		}

		_, err = x.LastInsertId()
		if err != nil {
			return err
		}
	}

	return nil
}

func addOption(tx *sql.Tx, o AddOption) (int64, error) {
	stmt, err := tx.Prepare("INSERT INTO OPTIONS(body, correct) VALUES(?, ?)")
	if err != nil {
		return -1, err
	}
	defer stmt.Close()

	x, err := stmt.Exec(o.Body, o.Correct)
	if err != nil {
		return -1, err
	}

	id, err := x.LastInsertId()
	if err != nil {
		return -1, err
	}

	return id, nil
}

func addQuestion(db *sql.DB, q AddQuestion) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO question_bodies(body) VALUES(?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	x, err := stmt.Exec(q.Body)
	if err != nil {
		return err
	}

	id, err := x.LastInsertId()
	if err != nil {
		return err
	}

	optionIds := make([]int64, len(q.Options))
	for index, option := range q.Options {
		optionId, err := addOption(tx, option)

		if err != nil {
			return err
		}

		optionIds[index] = optionId
	}

	err = addQuestionOptionRelations(tx, id, optionIds)
	if err == nil {
		tx.Commit()
	}

	return err
}

func getOption(db *sql.DB, id int64) (*Option, error) {
	row := db.QueryRow("SELECT body, correct FROM options WHERE id = ?", id)

	var body string
	var correct bool
	err := row.Scan(&body, &correct)

	if err != nil {
		return nil, err
	}

	return &Option{
		Id:      id,
		Body:    body,
		Correct: correct,
	}, nil
}

func getOptionsForQuestion(db *sql.DB, id int64) ([]Option, error) {
	rows, err := db.Query("SELECT option_id FROM questions WHERE question_id = ? ORDER BY option_order ASC", id)
	if err != nil {
		return nil, err
	}

	options := make([]Option, 0)

	for rows.Next() {
		var option_id int64
		err = rows.Scan(&option_id)

		if err != nil {
			return nil, err
		}

		option, err := getOption(db, option_id)
		if err != nil {
			return nil, err
		}

		options = append(options, *option)
	}
	return options, nil
}

func getQuestion(db *sql.DB, id int64) (*Question, error) {
	rows := db.QueryRow("SELECT body FROM question_bodies WHERE id = ?", id)

	var body string
	err := rows.Scan(&body)

	if err != nil {
		return nil, err
	}

	options, err := getOptionsForQuestion(db, id)
	if err != nil {
		return nil, err
	}

	return &Question{
		Id:      id,
		Body:    body,
		Options: options,
	}, nil
}

func getQuestions(db *sql.DB) ([]Question, error) {
	rows, err := db.Query("SELECT id, body FROM question_bodies")
	if err != nil {
		return nil, err
	}

	questions := make([]Question, 0)

	for rows.Next() {
		var id int64
		var body string
		err = rows.Scan(&id, &body)

		if err != nil {
			return nil, err
		}

		options, err := getOptionsForQuestion(db, id)
		if err != nil {
			return nil, err
		}

		questions = append(questions, Question{
			Id:      id,
			Body:    body,
			Options: options,
		})
	}

	return questions, nil
}

func deleteQuestionOptionRelation(tx *sql.Tx, questionId int64, optionId int64) error {
	stmt, err := tx.Prepare("DELETE FROM questions WHERE question_id = ? AND option_id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(questionId, optionId)
	return err
}

func deleteQuestionOptionRelations(tx *sql.Tx, questionId int64) error {
	stmt, err := tx.Prepare("DELETE FROM questions WHERE question_id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(questionId)
	return err
}

func deleteOption(tx *sql.Tx, o Option) error {
	stmt, err := tx.Prepare("DELETE FROM options WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(o.Id)
	return err
}

func deleteQuestion(db *sql.DB, q Question) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("DELETE FROM question_bodies WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	x, err := stmt.Exec(q.Id)
	if err != nil {
		return err
	}

	id, err := x.LastInsertId()
	if err != nil {
		return err
	}

	optionIds := make([]int64, len(q.Options))
	for index, option := range q.Options {
		err := deleteOption(tx, option)

		if err != nil {
			return err
		}

		optionIds[index] = option.Id
	}

	err = deleteQuestionOptionRelations(tx, id)

	if err == nil {
		tx.Commit()
	}

	return err
}

func updateQuestion(db *sql.DB, question Question) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("UPDATE question_bodies SET body = ? where id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(question.Body, question.Id)
	if err != nil {
		return err
	}

	// we could probably do something smarter for the update logic
	// and detect if the amount of options did change, but for simplicity
	// I'll just delete all of the options and re-add them
	deleteQuestionOptionRelations(tx, question.Id)

	optionIds := make([]int64, len(question.Options))
	for index, option := range question.Options {
		err = deleteOption(tx, option)

		if err != nil {
			return err
		}

		optionId, err := addOption(tx, AddOption{
			Body:    option.Body,
			Correct: option.Correct,
		})

		if err != nil {
			return err
		}

		optionIds[index] = optionId
	}

	err = addQuestionOptionRelations(tx, question.Id, optionIds)
	if err == nil {
		tx.Commit()
	}

	return err
}

func getDb() (*sql.DB, error) {
	newDb := false
	if _, err := os.Stat("./data.sqlite3"); os.IsNotExist(err) {
		newDb = true
	}
	db, err := sql.Open("sqlite3", "./data.sqlite3")

	if newDb {
		err = initDb(db)
		if err != nil {
			return nil, err
		}

		err = addQuestion(db, AddQuestion{
			Body: "a",
			Options: []AddOption{
				{
					Body:    "b",
					Correct: true,
				},
				{
					Body:    "c",
					Correct: false,
				},
			},
		})
		if err != nil {
			return nil, err
		}
	}

	return db, err
}

type QuestionController struct {
	web.Controller
}

func (ctrl *QuestionController) Question() {
	db, err := getDb()
	if err != nil {
		ctrl.Controller.Ctx.Abort(500, err.Error())
		return
	}

	defer db.Close()

	id, err := strconv.ParseInt(ctrl.Controller.Ctx.Input.Param(":id"), 10, 64)
	if err != nil {
		ctrl.Controller.Ctx.Abort(400, err.Error())
		return
	}

	question, err := getQuestion(db, id)
	if err != nil {
		ctrl.Controller.Ctx.Abort(400, err.Error())
		return
	}

	json.NewEncoder(ctrl.Controller.Ctx.ResponseWriter).Encode(question)
}

func (ctrl *QuestionController) Questions() {
	db, err := getDb()
	if err != nil {
		ctrl.Controller.Ctx.Abort(500, err.Error())
		return
	}

	defer db.Close()
	questions, err := getQuestions(db)
	if err != nil {
		ctrl.Controller.Ctx.Abort(500, err.Error())
		return
	}

	json.NewEncoder(ctrl.Controller.Ctx.ResponseWriter).Encode(questions)
}

func (ctrl *QuestionController) AddQuestion() {
	db, err := getDb()
	if err != nil {
		ctrl.Controller.Ctx.Abort(500, err.Error())
		return
	}

	defer db.Close()

	body := ctrl.Ctx.Input.RequestBody

	var q AddQuestion
	err = json.Unmarshal(body, &q)
	if err != nil {
		ctrl.Controller.Ctx.Abort(400, err.Error())
		return
	}

	err = addQuestion(db, q)
	if err != nil {
		ctrl.Controller.Ctx.Abort(400, err.Error())
		return
	}

	ctrl.Controller.Ctx.WriteString("OK")
}

func (ctrl *QuestionController) UpdateQuestion() {
	db, err := getDb()
	if err != nil {
		ctrl.Controller.Ctx.Abort(500, err.Error())
		return
	}

	defer db.Close()

	body := ctrl.Ctx.Input.RequestBody

	var q Question
	err = json.Unmarshal(body, &q)
	if err != nil {
		ctrl.Controller.Ctx.Abort(400, err.Error())
		return
	}

	err = updateQuestion(db, q)
	if err != nil {
		ctrl.Controller.Ctx.Abort(400, err.Error())
		return
	}

	ctrl.Controller.Ctx.WriteString("OK")
}

func (ctrl *QuestionController) DeleteQuestion() {
	db, err := getDb()
	if err != nil {
		ctrl.Controller.Ctx.Abort(500, err.Error())
		return
	}

	defer db.Close()

	id, err := strconv.ParseInt(ctrl.Controller.Ctx.Input.Param(":id"), 10, 64)
	if err != nil {
		ctrl.Controller.Ctx.Abort(400, err.Error())
		return
	}

	question, err := getQuestion(db, id)
	if err != nil {
		ctrl.Controller.Ctx.Abort(400, err.Error())
		return
	}

	err = deleteQuestion(db, *question)
	if err != nil {
		ctrl.Controller.Ctx.Abort(400, err.Error())
		return
	}

	ctrl.Controller.Ctx.WriteString("OK")
}

func main() {
	web.BConfig.Listen.HTTPPort = 8000
	if value, found := os.LookupEnv("PORT"); found {
		port, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			log.Fatal(err.Error())
			return
		}
		web.BConfig.Listen.HTTPPort = int(port)
	}
	web.BConfig.CopyRequestBody = true

	ctrl := &QuestionController{}

	web.Router("/question/:id", ctrl, "get:Question")
	web.Router("/question/:id", ctrl, "delete:DeleteQuestion")
	web.Router("/question", ctrl, "get:Questions")
	web.Router("/question", ctrl, "post:AddQuestion")
	web.Router("/question", ctrl, "put:UpdateQuestion")

	web.Run()
}
