package models

import "time"

// Role defines the possible user roles
type Role string

const (
	SuperAdmin Role = "super_admin"
	Recruiter  Role = "recruiter"
	Applicant  Role = "applicant"
)

// User represents a user in the system
type User struct {
	ID        string   `json:"id"`
	Email     string   `json:"email"`
	Name      string   `json:"name"`
	Role      Role     `json:"role"`
	CompanyID string   `json:"company_id,omitempty"`
	Skills    []string `json:"skills,omitempty"`
	Resume    string   `json:"resume,omitempty"`
	Approved  bool     `json:"approved"`
}

// Company represents a company tied to a recruiter
type Company struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Logo        string `json:"logo,omitempty"`
	Approved    bool   `json:"approved"`
}

// Job represents a job posting
type Job struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Skills      []string  `json:"skills"`
	CompanyID   int       `json:"company_id"`
	PostedBy    string    `json:"posted_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// Application represents a job application
type Application struct {
	ID          int       `json:"id"`
	JobID       int       `json:"job_id"`
	ApplicantID string    `json:"applicant_id"`
	Resume      string    `json:"resume"`
	Status      string    `json:"status"`
	AppliedAt   time.Time `json:"applied_at"`
	JobTitle    string    `json:"job_title,omitempty"` // For recruiter view
}

// Interview represents an interview request
type Interview struct {
	ID          int       `json:"id"`
	JobID       int       `json:"job_id"`
	ApplicantID string    `json:"applicant_id"`
	RecruiterID string    `json:"recruiter_id"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Status      string    `json:"status"`
}

// UserWithCompany is used for admin approval view
type UserWithCompany struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	Name      string  `json:"name"`
	Role      Role    `json:"role"`
	CompanyID string  `json:"company_id"`
	Company   Company `json:"company"`
}
