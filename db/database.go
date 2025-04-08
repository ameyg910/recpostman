package db

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"rec_postman/models"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

var DB *sql.DB

func InitDB() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file: ", err)
	}

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		log.Fatal("DATABASE_URL not set in .env")
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Error connecting to database: ", err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatal("Error pinging database: ", err)
	}

	DB = db
	log.Println("Database connection established")

	// Create tables
	userQuery := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT UNIQUE NOT NULL,
		name TEXT,
		role TEXT,
		company_id TEXT,
		skills JSONB,
		resume TEXT,
		approved BOOLEAN DEFAULT FALSE
	);`
	_, err = DB.Exec(userQuery)
	if err != nil {
		log.Fatal("Error creating users table: ", err)
	}

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

	jobQuery := `
	CREATE TABLE IF NOT EXISTS jobs (
		id SERIAL PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT,
		skills JSONB,
		company_id INTEGER REFERENCES companies(id),
		posted_by TEXT REFERENCES users(id),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	_, err = DB.Exec(jobQuery)
	if err != nil {
		log.Fatal("Error creating jobs table: ", err)
	}

	appQuery := `
	CREATE TABLE IF NOT EXISTS applications (
		id SERIAL PRIMARY KEY,
		job_id INTEGER REFERENCES jobs(id),
		applicant_id TEXT REFERENCES users(id),
		resume TEXT,
		status TEXT DEFAULT 'pending',
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	_, err = DB.Exec(appQuery)
	if err != nil {
		log.Fatal("Error creating applications table: ", err)
	}

	interviewQuery := `
	CREATE TABLE IF NOT EXISTS interviews (
		id SERIAL PRIMARY KEY,
		job_id INTEGER REFERENCES jobs(id),
		applicant_id TEXT REFERENCES users(id),
		recruiter_id TEXT REFERENCES users(id),
		scheduled_at TIMESTAMP,
		status TEXT DEFAULT 'requested'
	);`
	_, err = DB.Exec(interviewQuery)
	if err != nil {
		log.Fatal("Error creating interviews table: ", err)
	}

	followQuery := `
	CREATE TABLE IF NOT EXISTS company_followers (
		user_id TEXT REFERENCES users(id),
		company_id INTEGER REFERENCES companies(id),
		PRIMARY KEY (user_id, company_id)
	);`
	_, err = DB.Exec(followQuery)
	if err != nil {
		log.Fatal("Error creating company_followers table: ", err)
	}

	bookmarkQuery := `
    CREATE TABLE IF NOT EXISTS job_bookmarks (
        user_id TEXT REFERENCES users(id),
        job_id INTEGER REFERENCES jobs(id),
        PRIMARY KEY (user_id, job_id)
    );`
	_, err = DB.Exec(bookmarkQuery)
	if err != nil {
		log.Fatal("Error creating job_bookmarks table: ", err)
	}
}

func GetUser(id string) (*models.User, error) {
	user := &models.User{}
	var skillsJSON []byte
	var resume sql.NullString

	err := DB.QueryRow("SELECT id, email, name, role, company_id, skills, resume, approved FROM users WHERE id = $1", id).
		Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.CompanyID, &skillsJSON, &resume, &user.Approved)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if skillsJSON != nil {
		err = json.Unmarshal(skillsJSON, &user.Skills)
		if err != nil {
			return nil, err
		}
	}

	if resume.Valid {
		user.Resume = resume.String
	} else {
		user.Resume = ""
	}

	return user, nil
}

func SaveUser(user *models.User) error {
	skillsJSON, err := json.Marshal(user.Skills)
	if err != nil {
		return err
	}
	_, err = DB.Exec(`
		INSERT INTO users (id, email, name, role, company_id, skills, resume, approved)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE
		SET email = $2, name = $3, role = $4, company_id = $5, skills = $6, resume = $7, approved = $8`,
		user.ID, user.Email, user.Name, user.Role, user.CompanyID, skillsJSON, user.Resume, user.Approved)
	return err
}

func SaveCompany(company *models.Company) (int, error) {
	var id int
	err := DB.QueryRow(`
		INSERT INTO companies (title, description, logo, approved)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		company.Title, company.Description, company.Logo, company.Approved).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func GetCompany(id int) (*models.Company, error) {
	company := &models.Company{}
	err := DB.QueryRow("SELECT id, title, description, logo, approved FROM companies WHERE id = $1", id).
		Scan(&company.ID, &company.Title, &company.Description, &company.Logo, &company.Approved)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return company, nil
}

func SaveJob(job *models.Job) (int, error) {
	skillsJSON, err := json.Marshal(job.Skills)
	if err != nil {
		return 0, err
	}
	var id int
	err = DB.QueryRow(`
		INSERT INTO jobs (title, description, skills, company_id, posted_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		job.Title, job.Description, skillsJSON, job.CompanyID, job.PostedBy).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func GetJobsByRecruiter(recruiterID string) ([]models.Job, error) {
	rows, err := DB.Query("SELECT id, title, description, skills, company_id, posted_by, created_at FROM jobs WHERE posted_by = $1", recruiterID)
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
		if skillsJSON != nil {
			err = json.Unmarshal(skillsJSON, &job.Skills)
			if err != nil {
				return nil, err
			}
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func GetAllJobs() ([]models.Job, error) {
	rows, err := DB.Query("SELECT id, title, description, skills, company_id, posted_by, created_at FROM jobs")
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
		if skillsJSON != nil {
			err = json.Unmarshal(skillsJSON, &job.Skills)
			if err != nil {
				return nil, err
			}
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func SaveApplication(application *models.Application) (int, error) {
	var id int
	err := DB.QueryRow(`
		INSERT INTO applications (job_id, applicant_id, resume, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		application.JobID, application.ApplicantID, application.Resume, application.Status).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func GetApplicationsByApplicant(applicantID string) ([]models.Application, error) {
	rows, err := DB.Query("SELECT id, job_id, applicant_id, resume, status, applied_at FROM applications WHERE applicant_id = $1", applicantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var applications []models.Application
	for rows.Next() {
		var app models.Application
		var resume sql.NullString
		err := rows.Scan(&app.ID, &app.JobID, &app.ApplicantID, &resume, &app.Status, &app.AppliedAt)
		if err != nil {
			return nil, err
		}
		if resume.Valid {
			app.Resume = resume.String
		} else {
			app.Resume = ""
		}
		applications = append(applications, app)
	}
	return applications, nil
}
func UpdateApplicationStatus(applicationID, status string) error {
	_, err := DB.Exec("UPDATE applications SET status = $1 WHERE id = $2", status, applicationID)
	return err
}
func GetUserEmail(userID string) (string, error) {
	var email string
	err := DB.QueryRow("SELECT email FROM users WHERE id = $1", userID).Scan(&email)
	if err != nil {
		return "", err
	}
	return email, nil
}

func GetApplicationsByRecruiter(recruiterID string) ([]models.Application, error) {
	rows, err := DB.Query(`
        SELECT a.id, a.job_id, a.applicant_id, a.resume, a.status, j.title 
        FROM applications a 
        JOIN jobs j ON a.job_id = j.id 
        WHERE j.posted_by = $1`, recruiterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var applications []models.Application
	for rows.Next() {
		var app models.Application
		var resume sql.NullString // Handle nullable resume column
		var jobTitle string
		if err := rows.Scan(&app.ID, &app.JobID, &app.ApplicantID, &resume, &app.Status, &jobTitle); err != nil {
			return nil, err
		}
		// Safely convert sql.NullString to string
		if resume.Valid {
			app.Resume = resume.String
		} else {
			app.Resume = "" // Default to empty string if NULL
		}
		app.JobTitle = jobTitle
		applications = append(applications, app)
	}
	return applications, nil
}

func GetRecommendedJobs(skills []string) ([]models.Job, error) {
	rows, err := DB.Query("SELECT id, title, description, skills, company_id, posted_by, created_at FROM jobs")
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
		if skillsJSON != nil {
			err = json.Unmarshal(skillsJSON, &job.Skills)
			if err != nil {
				return nil, err
			}
		}
		for _, skill := range skills {
			for _, jobSkill := range job.Skills {
				if skill == jobSkill {
					jobs = append(jobs, job)
					break
				}
			}
		}
	}
	return jobs, nil
}

func SearchApplicantsBySkills(skills []string) ([]models.User, error) {
	rows, err := DB.Query("SELECT id, email, name, skills, resume FROM users WHERE role = 'applicant'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var applicants []models.User
	for rows.Next() {
		var user models.User
		var skillsJSON []byte
		var resume sql.NullString
		err := rows.Scan(&user.ID, &user.Email, &user.Name, &skillsJSON, &resume)
		if err != nil {
			return nil, err
		}
		if skillsJSON != nil {
			err = json.Unmarshal(skillsJSON, &user.Skills)
			if err != nil {
				return nil, err
			}
		}
		if resume.Valid {
			user.Resume = resume.String
		} else {
			user.Resume = ""
		}
		for _, skill := range skills {
			for _, userSkill := range user.Skills {
				if skill == userSkill {
					applicants = append(applicants, user)
					break
				}
			}
		}
	}
	return applicants, nil
}

func SaveInterview(interview *models.Interview) (int, error) {
	var id int
	err := DB.QueryRow(`
		INSERT INTO interviews (job_id, applicant_id, recruiter_id, scheduled_at, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		interview.JobID, interview.ApplicantID, interview.RecruiterID, interview.ScheduledAt, interview.Status).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func GetInterviewsByApplicant(applicantID string) ([]models.Interview, error) {
	rows, err := DB.Query("SELECT id, job_id, applicant_id, recruiter_id, scheduled_at, status FROM interviews WHERE applicant_id = $1", applicantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var interviews []models.Interview
	for rows.Next() {
		var interview models.Interview
		err := rows.Scan(&interview.ID, &interview.JobID, &interview.ApplicantID, &interview.RecruiterID, &interview.ScheduledAt, &interview.Status)
		if err != nil {
			return nil, err
		}
		interviews = append(interviews, interview)
	}
	return interviews, nil
}

func UpdateInterviewStatus(id int, status string, alternativeTime time.Time) error {
	if alternativeTime.IsZero() {
		_, err := DB.Exec("UPDATE interviews SET status = $1 WHERE id = $2", status, id)
		return err
	}
	_, err := DB.Exec("UPDATE interviews SET status = $1, scheduled_at = $2 WHERE id = $3", status, alternativeTime, id)
	return err
}

func GetInterview(id int) (*models.Interview, error) {
	interview := &models.Interview{}
	err := DB.QueryRow("SELECT id, job_id, applicant_id, recruiter_id, scheduled_at, status FROM interviews WHERE id = $1", id).
		Scan(&interview.ID, &interview.JobID, &interview.ApplicantID, &interview.RecruiterID, &interview.ScheduledAt, &interview.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return interview, nil
}

func GetJob(id int) (*models.Job, error) {
	job := &models.Job{}
	var skillsJSON []byte
	err := DB.QueryRow("SELECT id, title, description, skills, company_id, posted_by, created_at FROM jobs WHERE id = $1", id).
		Scan(&job.ID, &job.Title, &job.Description, &skillsJSON, &job.CompanyID, &job.PostedBy, &job.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if skillsJSON != nil {
		err = json.Unmarshal(skillsJSON, &job.Skills)
		if err != nil {
			return nil, err
		}
	}
	return job, nil
}

func FollowCompany(userID string, companyID int) error {
	_, err := DB.Exec(`
		INSERT INTO company_followers (user_id, company_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, company_id) DO NOTHING`,
		userID, companyID)
	return err
}

func GetFollowedCompanies(userID string) ([]models.Company, error) {
	rows, err := DB.Query(`
		SELECT c.id, c.title, c.description, c.logo, c.approved
		FROM companies c
		JOIN company_followers cf ON c.id = cf.company_id
		WHERE cf.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var companies []models.Company
	for rows.Next() {
		var company models.Company
		err := rows.Scan(&company.ID, &company.Title, &company.Description, &company.Logo, &company.Approved)
		if err != nil {
			return nil, err
		}
		companies = append(companies, company)
	}
	return companies, nil
}

func GetCompanyFollowers(companyID int) ([]models.User, error) {
	rows, err := DB.Query(`
		SELECT u.id, u.email, u.name, u.role, u.company_id, u.skills, u.resume, u.approved
		FROM users u
		JOIN company_followers cf ON u.id = cf.user_id
		WHERE cf.company_id = $1`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		var skillsJSON []byte
		var resume sql.NullString
		err := rows.Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.CompanyID, &skillsJSON, &resume, &user.Approved)
		if err != nil {
			return nil, err
		}
		if skillsJSON != nil {
			err = json.Unmarshal(skillsJSON, &user.Skills)
			if err != nil {
				return nil, err
			}
		}
		if resume.Valid {
			user.Resume = resume.String
		} else {
			user.Resume = ""
		}
		users = append(users, user)
	}
	return users, nil
}
func GetApplication(id string) (*models.Application, error) {
	application := &models.Application{}
	err := DB.QueryRow(`
		SELECT id, job_id, applicant_id, resume, status 
		FROM applications 
		WHERE id = $1`, id).Scan(&application.ID, &application.JobID, &application.ApplicantID, &application.Resume, &application.Status)
	if err != nil {
		return nil, err
	}
	return application, nil
}
func GetUnapprovedRecruitersWithCompanies() ([]models.UserWithCompany, error) {
	rows, err := DB.Query(`
		SELECT u.id, u.email, u.name, u.role, u.company_id, c.id, c.title, c.description, c.logo
		FROM users u
		JOIN companies c ON u.company_id = c.id::text
		WHERE u.role = 'recruiter' AND u.approved = FALSE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recruiters []models.UserWithCompany
	for rows.Next() {
		var r models.UserWithCompany
		err := rows.Scan(&r.ID, &r.Email, &r.Name, &r.Role, &r.CompanyID, &r.Company.ID, &r.Company.Title, &r.Company.Description, &r.Company.Logo)
		if err != nil {
			return nil, err
		}
		r.Company.Approved = false
		recruiters = append(recruiters, r)
	}
	return recruiters, nil
}
func BookmarkJob(userID string, jobID int) error {
	_, err := DB.Exec(`
        INSERT INTO job_bookmarks (user_id, job_id)
        VALUES ($1, $2)
        ON CONFLICT (user_id, job_id) DO NOTHING`, // Prevents duplicate bookmarks
		userID, jobID)
	return err
}

func GetBookmarkedJobs(userID string) ([]models.Job, error) {
	rows, err := DB.Query(`
        SELECT j.id, j.title, j.description, j.skills, j.company_id, j.posted_by, j.created_at
        FROM jobs j
        JOIN job_bookmarks jb ON j.id = jb.job_id
        WHERE jb.user_id = $1`, userID)
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
		if skillsJSON != nil {
			err = json.Unmarshal(skillsJSON, &job.Skills)
			if err != nil {
				return nil, err
			}
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}
