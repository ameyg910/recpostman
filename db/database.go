package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"rec_postman/models"

	_ "github.com/lib/pq"
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

	// Users table
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

	// Companies table
	companyQuery := `
    CREATE TABLE IF NOT EXISTS companies (
        id SERIAL PRIMARY KEY,
        title TEXT NOT NULL,
        description TEXT,
        logo TEXT,
        approved BOOLEAN DEFAULT FALSE
    );`
	_, err = DB.Exec(companyQuery)
	if err != nil {
		log.Fatal("Error creating companies table: ", err)
	}

	// Add foreign key constraint (optional, for referential integrity)
	alterQuery := `
    ALTER TABLE users ADD COLUMN IF NOT EXISTS company_id TEXT REFERENCES companies(id) ON DELETE SET NULL;`
	_, err = DB.Exec(alterQuery)
	if err != nil {
		log.Println("Warning: Could not add foreign key constraint:", err)
	}

	log.Println("Connected to PostgreSQL and initialized tables")
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
	approved := user.Role != models.Recruiter
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

func SaveCompany(company *models.Company) (int, error) {
	query := `
    INSERT INTO companies (title, description, logo, approved)
    VALUES ($1, $2, $3, $4)
    RETURNING id;`
	var id int
	err := DB.QueryRow(query, company.Title, company.Description, company.Logo, company.Approved).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func GetCompany(id int) (*models.Company, error) {
	query := `SELECT id, title, description, logo, approved FROM companies WHERE id = $1;`
	row := DB.QueryRow(query, id)

	var company models.Company
	err := row.Scan(&company.ID, &company.Title, &company.Description, &company.Logo, &company.Approved)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &company, nil
}

func GetUnapprovedRecruitersWithCompanies() ([]struct {
	UserID       string
	Email        string
	Name         string
	CompanyID    int
	CompanyTitle string
}, error) {
	query := `
    SELECT u.id, u.email, u.name, u.company_id, c.title
    FROM users u
    JOIN companies c ON u.company_id = c.id
    WHERE u.role = 'recruiter' AND u.approved = FALSE AND c.approved = FALSE;`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recruiters []struct {
		UserID       string
		Email        string
		Name         string
		CompanyID    int
		CompanyTitle string
	}
	for rows.Next() {
		var r struct {
			UserID       string
			Email        string
			Name         string
			CompanyID    int
			CompanyTitle string
		}
		err := rows.Scan(&r.UserID, &r.Email, &r.Name, &r.CompanyID, &r.CompanyTitle)
		if err != nil {
			return nil, err
		}
		recruiters = append(recruiters, r)
	}
	return recruiters, nil
}

func SearchApplicantsBySkills(skills []string) ([]models.User, error) {
	query := `SELECT id, email, name, role, skills, company_id, approved 
             FROM users 
             WHERE role = 'applicant' AND skills @> $1::jsonb;`
	skillsJSON, _ := json.Marshal(skills)
	rows, err := DB.Query(query, skillsJSON)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var applicants []models.User
	for rows.Next() {
		var user models.User
		var skillsJSON []byte
		err := rows.Scan(&user.ID, &user.Email, &user.Name, &user.Role, &skillsJSON, &user.CompanyID, &user.Approved)
		if err != nil {
			return nil, err
		}
		if len(skillsJSON) > 0 {
			json.Unmarshal(skillsJSON, &user.Skills)
		}
		applicants = append(applicants, user)
	}
	return applicants, nil
}
