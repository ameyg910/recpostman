package db

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "os"

    _ "github.com/lib/pq"
    "rec_postman/models"
)

var DB *sql.DB

func InitDB() {
    connStr := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable",
        os.Getenv("DB_HOST"),
        os.Getenv("DB_PORT"),
        os.Getenv("DB_NAME"),
        os.Getenv("DB_USER"),
        os.Getenv("DB_PASSWORD"),
    )

    var err error
    DB, err = sql.Open("postgres", connStr)
    if err != nil {
        log.Fatal("Error opening database: ", err)
    }
    if err = DB.Ping(); err != nil {
        log.Fatal("Error connecting to database: ", err)
    }

    query := `
    CREATE TABLE IF NOT EXISTS users (
        id TEXT PRIMARY KEY,
        email TEXT UNIQUE NOT NULL,
        name TEXT NOT NULL,
        role TEXT NOT NULL,
        skills JSONB,
        company_id TEXT,
        approved BOOLEAN DEFAULT FALSE
    );`
    _, err = DB.Exec(query)
    if err != nil {
        log.Fatal("Error creating users table: ", err)
    }

    alterQuery := `
    ALTER TABLE users ADD COLUMN IF NOT EXISTS approved BOOLEAN DEFAULT FALSE;`
    _, err = DB.Exec(alterQuery)
    if err != nil {
        log.Fatal("Error adding approved column: ", err)
    }

    log.Println("Connected to PostgreSQL and initialized users table")
}

func SaveUser(user *models.User) error {
    query := `
    INSERT INTO users (id, email, name, role, skills, company_id, approved)
    VALUES ($1, $2, $3, $4, $5, $6, $7)
    ON CONFLICT (id) DO UPDATE
    SET email = EXCLUDED.email,
        name = EXCLUDED.name,
        role = EXCLUDED.role,
        skills = EXCLUDED.skills,
        company_id = EXCLUDED.company_id,
        approved = EXCLUDED.approved;`
    skillsJSON, _ := json.Marshal(user.Skills)
    approved := user.Role != models.Recruiter // Only recruiters need approval
    _, err := DB.Exec(query, user.ID, user.Email, user.Name, user.Role, skillsJSON, user.CompanyID, approved)
    return err
}

func GetUser(id string) (*models.User, error) {
    query := `SELECT id, email, name, role, skills, company_id, approved FROM users WHERE id = $1;`
    row := DB.QueryRow(query, id)

    var user models.User
    var skillsJSON []byte
    var approved bool
    err := row.Scan(&user.ID, &user.Email, &user.Name, &user.Role, &skillsJSON, &user.CompanyID, &approved)
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, nil
        }
        return nil, err
    }

    if len(skillsJSON) > 0 {
        json.Unmarshal(skillsJSON, &user.Skills)
    }
    return &user, nil
}