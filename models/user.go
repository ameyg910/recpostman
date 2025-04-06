package models

import "time"

type Role string

const (
	SuperAdmin Role = "super_admin"
	Recruiter  Role = "recruiter"
	Applicant  Role = "applicant"
)

type User struct {
	ID        string
	Email     string
	Name      string
	Role      Role
	Skills    []string
	CompanyID string
	Approved  bool
	Resume    string
}

type Company struct {
	ID          int
	Title       string
	Description string
	Logo        string
	Approved    bool
}

type Job struct {
	ID          int
	Title       string
	Description string
	Skills      []string
	CompanyID   int
	PostedBy    string
	CreatedAt   time.Time // Changed back from PostedAt to match original
}

type Application struct {
	ID          int
	JobID       int
	ApplicantID string
	Resume      string // Added
	Status      string
	AppliedAt   time.Time
	JobTitle    string // Added for display purposes
}

type Interview struct {
	ID          int
	JobID       int
	ApplicantID string
	RecruiterID string
	Status      string
	ScheduledAt time.Time
}

type UserWithCompany struct {
	ID        string
	Email     string
	Name      string
	Role      Role
	CompanyID string
	Company   Company
}
