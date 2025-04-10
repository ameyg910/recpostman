package models

import "time"

type Role string

const (
	SuperAdmin Role = "super_admin"
	Recruiter  Role = "recruiter"
	Applicant  Role = "applicant"
)

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

type Company struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Logo        string `json:"logo,omitempty"`
	Approved    bool   `json:"approved"`
}

type Job struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Skills      []string  `json:"skills"`
	CompanyID   int       `json:"company_id"`
	PostedBy    string    `json:"posted_by"`
	CreatedAt   time.Time `json:"created_at"`
}

type Application struct {
	ID          int       `json:"id"`
	JobID       int       `json:"job_id"`
	ApplicantID string    `json:"applicant_id"`
	Resume      string    `json:"resume"`
	Status      string    `json:"status"`
	AppliedAt   time.Time `json:"applied_at"`
	JobTitle    string    `json:"job_title,omitempty"`
}

type Interview struct {
	ID          int       `json:"id"`
	JobID       int       `json:"job_id"`
	ApplicantID string    `json:"applicant_id"`
	RecruiterID string    `json:"recruiter_id"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Status      string    `json:"status"`
}

type UserWithCompany struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	Name      string  `json:"name"`
	Role      Role    `json:"role"`
	CompanyID string  `json:"company_id"`
	Company   Company `json:"company"`
}
