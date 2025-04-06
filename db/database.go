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

	// Users table with resume column
	query := `
    CREATE TABLE IF NOT EXISTS users (
        id TEXT PRIMARY KEY,
        email TEXT UNIQUE NOT NULL,
        name TEXT NOT NULL,
        role TEXT NOT NULL,
        skills JSONB,
        company_id TEXT,
        approved BOOLEAN DEFAULT FALSE,
        resume TEXT
    );`
	_, err = DB.Exec(query)
	if err != nil {
		log.Fatal("Error creating users table: ", err)
	}
	alterQuery := `
    DO $$
    BEGIN
        IF NOT EXISTS (
            SELECT 1 
            FROM information_schema.columns 
            WHERE table_name = 'users' 
            AND column_name = 'resume'
        ) THEN
            ALTER TABLE users ADD COLUMN resume TEXT;
        END IF;
    END;
    $$;`
	_, err = DB.Exec(alterQuery)
	if err != nil {
		log.Println("Error adding resume column (might already exist):", err)
	} else {
		log.Println("Added resume column to users table")
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

	// Jobs table
	jobQuery := `
    CREATE TABLE IF NOT EXISTS jobs (
        id SERIAL PRIMARY KEY,
        title TEXT NOT NULL,
        description TEXT NOT NULL,
        skills JSONB,
        company_id INTEGER REFERENCES companies(id),
        posted_by TEXT REFERENCES users(id),
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );`
	_, err = DB.Exec(jobQuery)
	if err != nil {
		log.Fatal("Error creating jobs table: ", err)
	}

	// Applications table
	appQuery := `
    CREATE TABLE IF NOT EXISTS applications (
        id SERIAL PRIMARY KEY,
        job_id INTEGER REFERENCES jobs(id),
        applicant_id TEXT REFERENCES users(id),
        status TEXT DEFAULT 'pending',
        applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );`
	_, err = DB.Exec(appQuery)
	if err != nil {
		log.Fatal("Error creating applications table: ", err)
	}

	// Interviews table
	interviewQuery := `
    CREATE TABLE IF NOT EXISTS interviews (
        id SERIAL PRIMARY KEY,
        job_id INTEGER REFERENCES jobs(id),
        applicant_id TEXT REFERENCES users(id),
        recruiter_id TEXT REFERENCES users(id),
        status TEXT DEFAULT 'requested',
        scheduled_at TIMESTAMP
    );`
	_, err = DB.Exec(interviewQuery)
	if err != nil {
		log.Fatal("Error creating interviews table: ", err)
	}

	log.Println("Connected to PostgreSQL and initialized tables")
}

func SaveUser(user *models.User) error {
	query := `
    INSERT INTO users (id, email, name, role, skills, company_id, approved, resume)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
    ON CONFLICT (id) DO UPDATE
    SET email = EXCLUDED.email,
        name = EXCLUDED.name,
        role = EXCLUDED.role,
        skills = EXCLUDED.skills,
        company_id = EXCLUDED.company_id,
        approved = EXCLUDED.approved,
        resume = EXCLUDED.resume;`
	skillsJSON, _ := json.Marshal(user.Skills)
	approved := user.Role != models.Recruiter // Default approved to true unless recruiter
	_, err := DB.Exec(query, user.ID, user.Email, user.Name, user.Role, skillsJSON, user.CompanyID, approved, user.Resume)
	return err
}

func GetUser(id string) (*models.User, error) {
	query := `SELECT id, email, name, role, skills, company_id, approved, resume FROM users WHERE id = $1;`
	row := DB.QueryRow(query, id)

	var user models.User
	var skillsJSON []byte
	err := row.Scan(&user.ID, &user.Email, &user.Name, &user.Role, &skillsJSON, &user.CompanyID, &user.Approved, &user.Resume)
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

func GetUnapprovedRecruitersWithCompanies() ([]models.UserWithCompany, error) {
	query := `
    SELECT u.id, u.email, u.name, u.role, u.company_id, c.id, c.title, c.description, c.logo, c.approved
    FROM users u
    JOIN companies c ON u.company_id = c.id::TEXT
    WHERE u.role = 'recruiter' AND u.approved = FALSE AND c.approved = FALSE;`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recruiters []models.UserWithCompany
	for rows.Next() {
		var r models.UserWithCompany
		err := rows.Scan(&r.ID, &r.Email, &r.Name, &r.Role, &r.CompanyID, &r.Company.ID, &r.Company.Title, &r.Company.Description, &r.Company.Logo, &r.Company.Approved)
		if err != nil {
			return nil, err
		}
		recruiters = append(recruiters, r)
	}
	return recruiters, nil
}

func SearchApplicantsBySkills(skills []string) ([]models.User, error) {
	query := `SELECT id, email, name, role, skills, company_id, approved, resume 
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
		err := rows.Scan(&user.ID, &user.Email, &user.Name, &user.Role, &skillsJSON, &user.CompanyID, &user.Approved, &user.Resume)
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

func SaveJob(job *models.Job) (int, error) {
	query := `
    INSERT INTO jobs (title, description, skills, company_id, posted_by)
    VALUES ($1, $2, $3, $4, $5)
    RETURNING id;`
	skillsJSON, _ := json.Marshal(job.Skills)
	var id int
	err := DB.QueryRow(query, job.Title, job.Description, skillsJSON, job.CompanyID, job.PostedBy).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func GetJobsByRecruiter(recruiterID string) ([]models.Job, error) {
	query := `
    SELECT id, title, description, skills, company_id, posted_by, created_at
    FROM jobs
    WHERE posted_by = $1;`
	rows, err := DB.Query(query, recruiterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		var job models.Job
		var skillsJSON []byte
		err := rows.Scan(&job.ID, &job.Title, &job.Description, &skillsJSON, &job.CompanyID, &job.PostedBy, &job.CreatedAt)
		if err != nil {
			return nil, err
		}
		if len(skillsJSON) > 0 {
			json.Unmarshal(skillsJSON, &job.Skills)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func GetAllJobs() ([]models.Job, error) {
	query := `
    SELECT id, title, description, skills, company_id, posted_by, created_at
    FROM jobs;`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		var job models.Job
		var skillsJSON []byte
		err := rows.Scan(&job.ID, &job.Title, &job.Description, &skillsJSON, &job.CompanyID, &job.PostedBy, &job.CreatedAt)
		if err != nil {
			return nil, err
		}
		if len(skillsJSON) > 0 {
			json.Unmarshal(skillsJSON, &job.Skills)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func GetRecommendedJobs(skills []string) ([]models.Job, error) {
	query := `
    SELECT id, title, description, skills, company_id, posted_by, created_at
    FROM jobs
    WHERE skills @> $1::jsonb;`
	skillsJSON, _ := json.Marshal(skills)
	rows, err := DB.Query(query, skillsJSON)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		var job models.Job
		var skillsJSON []byte
		err := rows.Scan(&job.ID, &job.Title, &job.Description, &skillsJSON, &job.CompanyID, &job.PostedBy, &job.CreatedAt)
		if err != nil {
			return nil, err
		}
		if len(skillsJSON) > 0 {
			json.Unmarshal(skillsJSON, &job.Skills)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func SaveApplication(application *models.Application) (int, error) {
	query := `
    INSERT INTO applications (job_id, applicant_id, status)
    VALUES ($1, $2, $3)
    RETURNING id;`
	var id int
	err := DB.QueryRow(query, application.JobID, application.ApplicantID, application.Status).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func GetApplicationsByApplicant(applicantID string) ([]models.Application, error) {
	query := `
    SELECT id, job_id, applicant_id, status, applied_at
    FROM applications
    WHERE applicant_id = $1;`
	rows, err := DB.Query(query, applicantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var applications []models.Application
	for rows.Next() {
		var app models.Application
		err := rows.Scan(&app.ID, &app.JobID, &app.ApplicantID, &app.Status, &app.AppliedAt)
		if err != nil {
			return nil, err
		}
		applications = append(applications, app)
	}
	return applications, nil
}

func SaveInterview(interview *models.Interview) (int, error) {
	query := `
    INSERT INTO interviews (job_id, applicant_id, recruiter_id, status, scheduled_at)
    VALUES ($1, $2, $3, $4, $5)
    RETURNING id;`
	var id int
	err := DB.QueryRow(query, interview.JobID, interview.ApplicantID, interview.RecruiterID, interview.Status, interview.ScheduledAt).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func GetInterviewsByApplicant(applicantID string) ([]models.Interview, error) {
	query := `
    SELECT id, job_id, applicant_id, recruiter_id, status, scheduled_at
    FROM interviews
    WHERE applicant_id = $1;`
	rows, err := DB.Query(query, applicantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var interviews []models.Interview
	for rows.Next() {
		var interview models.Interview
		err := rows.Scan(&interview.ID, &interview.JobID, &interview.ApplicantID, &interview.RecruiterID, &interview.Status, &interview.ScheduledAt)
		if err != nil {
			return nil, err
		}
		interviews = append(interviews, interview)
	}
	return interviews, nil
}

func UpdateInterviewStatus(interviewID int, status string) error {
	query := `UPDATE interviews SET status = $1 WHERE id = $2;`
	_, err := DB.Exec(query, status, interviewID)
	return err
}
