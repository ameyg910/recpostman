package models

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
}