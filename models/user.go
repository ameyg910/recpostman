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
	CompanyID string   `json:"company_id,omitempty"` // Optional for applicants
	Skills    []string `json:"skills,omitempty"`     // Stored as JSONB in DB
	Resume    string   `json:"resume,omitempty"`     // Path to uploaded resume
	Approved  bool     `json:"approved"`
}

type Company struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Logo        string `json:"logo,omitempty"`
	Approved    bool   `json:"approved"`
}

type Job struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Skills      []string  `json:"skills"` // Stored as JSONB in DB
	CompanyID   int       `json:"company_id"`
	PostedBy    string    `json:"posted_by"` // User ID of the recruiter
	CreatedAt   time.Time `json:"created_at"`
}

type Application struct {
	ID          int       `json:"id"`
	JobID       int       `json:"job_id"`
	ApplicantID string    `json:"applicant_id"`
	Resume      string    `json:"resume,omitempty"`
	Status      string    `json:"status"` // e.g., "pending", "accepted", "rejected"
	AppliedAt   time.Time `json:"applied_at"`
	JobTitle    string    `json:"job_title,omitempty"` // For display purposes, not stored in DB
}

type Interview struct {
	ID          int       `json:"id"`
	JobID       int       `json:"job_id"`
	ApplicantID string    `json:"applicant_id"`
	RecruiterID string    `json:"recruiter_id"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Status      string    `json:"status"` // e.g., "requested", "accepted", "declined"
	MeetLink    string    `json:"meet_link,omitempty"`
}

type UserWithCompany struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	Name      string  `json:"name"`
	Role      Role    `json:"role"`
	CompanyID string  `json:"company_id"`
	Company   Company `json:"company"`
}
