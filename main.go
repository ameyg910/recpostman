package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"rec_postman/db"
	"rec_postman/models"
	"strconv"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	googleOauthConfig *oauth2.Config
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file: ", err)
	}

	googleOauthConfig = &oauth2.Config{
		RedirectURL:  "http://localhost:8080/auth/google/callback",
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint:     google.Endpoint,
	}

	if googleOauthConfig.ClientID == "" || googleOauthConfig.ClientSecret == "" {
		log.Fatal("GOOGLE_CLIENT_ID or GOOGLE_CLIENT_SECRET not set in .env")
	}

	db.InitDB()
}

func main() {
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")

	store := cookie.NewStore([]byte("secret-key"))
	r.Use(sessions.Sessions("mysession", store))

	r.Static("/uploads", "./uploads")

	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Welcome to the Recruitment Portal!")
	})
	r.GET("/auth/google/login", handleGoogleLogin)
	r.GET("/auth/google/callback", handleGoogleCallback)
	r.GET("/select-role", handleSelectRole)
	r.POST("/select-role", handleRoleSubmission)
	r.GET("/logout", handleLogout)

	r.GET("/dashboard", requireRole(models.SuperAdmin, models.Recruiter, models.Applicant), handleDashboard)
	r.POST("/recruiter/post-job", requireRole(models.Recruiter), handlePostJob)
	r.POST("/recruiter/search-applicants", requireRole(models.Recruiter), handleSearchApplicants)
	r.POST("/recruiter/request-interview", requireRole(models.Recruiter), handleRequestInterview)
	r.POST("/applicant/apply-job", requireRole(models.Applicant), handleApplyJob)
	r.POST("/applicant/upload-resume", requireRole(models.Applicant), handleUploadResume)
	r.POST("/applicant/update-interview", requireRole(models.Applicant), handleUpdateInterview)

	r.GET("/admin", requireRole(models.SuperAdmin), func(c *gin.Context) {
		c.String(http.StatusOK, "Super Admin panel")
	})
	r.GET("/admin/approve-recruiters", requireRole(models.SuperAdmin), handleApproveRecruiters)
	r.POST("/admin/approve-recruiter", requireRole(models.SuperAdmin), handleApproveRecruiter)

	r.Run(":8080")
}

func handleGoogleLogin(c *gin.Context) {
	url := googleOauthConfig.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "select_account"))
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func handleGoogleCallback(c *gin.Context) {
	code := c.Query("code")
	token, err := googleOauthConfig.Exchange(c, code)
	if err != nil {
		c.String(http.StatusBadRequest, "Failed to exchange token: "+err.Error())
		return
	}

	client := googleOauthConfig.Client(c, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		c.String(http.StatusBadRequest, "Failed to get user info: "+err.Error())
		return
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to read response: "+err.Error())
		return
	}

	var userInfo struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"given_name"`
	}
	if err := json.Unmarshal(data, &userInfo); err != nil {
		c.String(http.StatusInternalServerError, "Failed to parse user info: "+err.Error())
		return
	}

	user := models.User{
		ID:    userInfo.ID,
		Email: userInfo.Email,
		Name:  userInfo.Name,
	}

	if err := db.SaveUser(&user); err != nil {
		c.String(http.StatusInternalServerError, "Failed to save user: "+err.Error())
		return
	}

	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Save()

	c.Redirect(http.StatusFound, "/select-role?id="+user.ID)
}

func handleSelectRole(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id")
	if userID == nil {
		c.Redirect(http.StatusFound, "/auth/google/login")
		return
	}

	c.HTML(http.StatusOK, "select_role.html", gin.H{
		"UserID": userID,
	})
}

func handleRoleSubmission(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id")
	if userID == nil {
		c.Redirect(http.StatusFound, "/auth/google/login")
		return
	}

	role := c.PostForm("role")
	user, err := db.GetUser(userID.(string))
	if err != nil {
		c.String(http.StatusBadRequest, "User not found: "+err.Error())
		return
	}

	switch role {
	case "super_admin":
		user.Role = models.SuperAdmin
	case "recruiter":
		user.Role = models.Recruiter
		companyTitle := c.PostForm("company_title")
		companyDesc := c.PostForm("company_description")
		companyLogo := c.PostForm("company_logo")
		if companyTitle == "" {
			c.String(http.StatusBadRequest, "Company title is required for recruiters")
			return
		}
		company := models.Company{
			Title:       companyTitle,
			Description: companyDesc,
			Logo:        companyLogo,
			Approved:    false,
		}
		companyID, err := db.SaveCompany(&company)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to save company: "+err.Error())
			return
		}
		user.CompanyID = strconv.Itoa(companyID)
	case "applicant":
		user.Role = models.Applicant
		skills := c.PostFormArray("skills")
		if len(skills) == 0 {
			c.String(http.StatusBadRequest, "At least one skill is required for applicants")
			return
		}
		user.Skills = skills
	default:
		c.String(http.StatusBadRequest, "Invalid role")
		return
	}

	if err := db.SaveUser(user); err != nil {
		c.String(http.StatusInternalServerError, "Failed to update user: "+err.Error())
		return
	}

	c.Redirect(http.StatusFound, "/dashboard")
}

func handleDashboard(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id")
	if userID == nil {
		c.Redirect(http.StatusFound, "/auth/google/login")
		return
	}

	user, err := db.GetUser(userID.(string))
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch user: "+err.Error())
		return
	}

	switch user.Role {
	case models.Recruiter:
		var approved bool
		db.DB.QueryRow("SELECT approved FROM users WHERE id = $1", user.ID).Scan(&approved)
		if !approved {
			c.String(http.StatusForbidden, "Your recruiter account is pending approval.")
			return
		}
		companyID, _ := strconv.Atoi(user.CompanyID)
		company, err := db.GetCompany(companyID)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to fetch company: "+err.Error())
			return
		}
		jobs, err := db.GetJobsByRecruiter(user.ID)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to fetch jobs: "+err.Error())
			return
		}
		c.HTML(http.StatusOK, "recruiter_dashboard.html", gin.H{
			"Name":    user.Name,
			"Company": company,
			"Jobs":    jobs,
		})
	case models.Applicant:
		jobs, err := db.GetAllJobs()
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to fetch jobs: "+err.Error())
			return
		}
		applications, err := db.GetApplicationsByApplicant(user.ID)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to fetch applications: "+err.Error())
			return
		}
		recommendedJobs, err := db.GetRecommendedJobs(user.Skills)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to fetch recommended jobs: "+err.Error())
			return
		}
		interviews, err := db.GetInterviewsByApplicant(user.ID)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to fetch interviews: "+err.Error())
			return
		}
		c.HTML(http.StatusOK, "applicant_dashboard.html", gin.H{
			"Name":            user.Name,
			"Skills":          user.Skills,
			"Jobs":            jobs,
			"Applications":    applications,
			"RecommendedJobs": recommendedJobs,
			"Interviews":      interviews,
			"Resume":          user.Resume,
		})
	case models.SuperAdmin:
		c.HTML(http.StatusOK, "dashboard.html", gin.H{
			"Name": user.Name,
			"Role": string(user.Role),
		})
	}
}

func handlePostJob(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id")
	if userID == nil {
		c.Redirect(http.StatusFound, "/auth/google/login")
		return
	}

	user, err := db.GetUser(userID.(string))
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch user: "+err.Error())
		return
	}

	title := c.PostForm("title")
	description := c.PostForm("description")
	skills := c.PostFormArray("skills")
	if title == "" || description == "" || len(skills) == 0 {
		c.String(http.StatusBadRequest, "Title, description, and skills are required")
		return
	}

	companyID, _ := strconv.Atoi(user.CompanyID)
	job := models.Job{
		Title:       title,
		Description: description,
		Skills:      skills,
		CompanyID:   companyID,
		PostedBy:    user.ID,
	}

	_, err = db.SaveJob(&job)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to post job: "+err.Error())
		return
	}

	c.Redirect(http.StatusFound, "/dashboard")
}

func handleApplyJob(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id")
	if userID == nil {
		c.Redirect(http.StatusFound, "/auth/google/login")
		return
	}

	jobIDStr := c.PostForm("job_id")
	jobID, err := strconv.Atoi(jobIDStr)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid job ID")
		return
	}

	application := models.Application{
		JobID:       jobID,
		ApplicantID: userID.(string),
		Status:      "pending",
	}

	_, err = db.SaveApplication(&application)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to apply for job: "+err.Error())
		return
	}

	c.Redirect(http.StatusFound, "/dashboard")
}

func handleUploadResume(c *gin.Context) {
	log.Println("Entering handleUploadResume")
	session := sessions.Default(c)
	userID := session.Get("user_id")
	if userID == nil {
		log.Println("No user ID in session, redirecting to login")
		c.Redirect(http.StatusFound, "/auth/google/login")
		return
	}
	log.Println("User ID:", userID)

	file, err := c.FormFile("resume")
	if err != nil {
		log.Println("Failed to get resume file:", err)
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"Message": "Failed to get resume file: " + err.Error()})
		return
	}
	log.Println("File received:", file.Filename)

	uploadDir := "./uploads"
	if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
		log.Println("Creating upload directory")
		if err := os.Mkdir(uploadDir, 0755); err != nil {
			log.Println("Failed to create upload directory:", err)
			c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to create upload directory: " + err.Error()})
			return
		}
	}
	filename := filepath.Join(uploadDir, userID.(string)+"_"+file.Filename)
	log.Println("Saving file to:", filename)
	if err := c.SaveUploadedFile(file, filename); err != nil {
		log.Println("Failed to save file:", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to save resume: " + err.Error()})
		return
	}

	log.Println("Fetching user from DB")
	user, err := db.GetUser(userID.(string))
	if err != nil {
		log.Println("Failed to fetch user:", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to fetch user: " + err.Error()})
		return
	}
	user.Resume = filename
	log.Println("Updating user with resume:", filename)
	if err := db.SaveUser(user); err != nil {
		log.Println("Failed to update user:", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to update user with resume: " + err.Error()})
		return
	}

	log.Println("Redirecting to /dashboard")
	c.Redirect(http.StatusFound, "/dashboard")
}

func handleRequestInterview(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id")
	if userID == nil {
		c.Redirect(http.StatusFound, "/auth/google/login")
		return
	}

	applicantID := c.PostForm("applicant_id")
	jobIDStr := c.PostForm("job_id")
	scheduledAtStr := c.PostForm("scheduled_at") // Format: "2025-04-10 14:00"

	jobID, err := strconv.Atoi(jobIDStr)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid job ID")
		return
	}

	scheduledAt, err := time.Parse("2006-01-02 15:04", scheduledAtStr)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid scheduled time format (use YYYY-MM-DD HH:MM)")
		return
	}

	interview := models.Interview{
		JobID:       jobID,
		ApplicantID: applicantID,
		RecruiterID: userID.(string),
		Status:      "requested",
		ScheduledAt: scheduledAt,
	}

	_, err = db.SaveInterview(&interview)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to request interview: "+err.Error())
		return
	}

	c.Redirect(http.StatusFound, "/dashboard")
}

func handleUpdateInterview(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id")
	if userID == nil {
		c.Redirect(http.StatusFound, "/auth/google/login")
		return
	}

	interviewIDStr := c.PostForm("interview_id")
	status := c.PostForm("status") // "accepted" or "declined"
	interviewID, err := strconv.Atoi(interviewIDStr)
	if err != nil || (status != "accepted" && status != "declined") {
		c.String(http.StatusBadRequest, "Invalid interview ID or status")
		return
	}

	if err := db.UpdateInterviewStatus(interviewID, status); err != nil {
		c.String(http.StatusInternalServerError, "Failed to update interview: "+err.Error())
		return
	}

	c.Redirect(http.StatusFound, "/dashboard")
}

func requireRole(roles ...models.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		userID := session.Get("user_id")
		if userID == nil {
			c.Redirect(http.StatusFound, "/auth/google/login")
			c.Abort()
			return
		}

		user, err := db.GetUser(userID.(string))
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to fetch user: "+err.Error())
			c.Abort()
			return
		}

		for _, role := range roles {
			if user.Role == role {
				c.Next()
				return
			}
		}
		c.String(http.StatusForbidden, "Access denied: insufficient permissions")
		c.Abort()
	}
}

func handleApproveRecruiters(c *gin.Context) {
	recruiters, err := db.GetUnapprovedRecruitersWithCompanies()
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch recruiters: "+err.Error())
		return
	}

	c.HTML(http.StatusOK, "approve_recruiters.html", gin.H{
		"Recruiters": recruiters,
	})
}

func handleApproveRecruiter(c *gin.Context) {
	userID := c.PostForm("id")
	companyID := c.PostForm("company_id")
	_, err := db.DB.Exec("UPDATE users SET approved = TRUE WHERE id = $1 AND role = 'recruiter'", userID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to approve recruiter: "+err.Error())
		return
	}
	_, err = db.DB.Exec("UPDATE companies SET approved = TRUE WHERE id = $1", companyID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to approve company: "+err.Error())
		return
	}
	c.Redirect(http.StatusFound, "/admin/approve-recruiters")
}

func handleSearchApplicants(c *gin.Context) {
	skills := c.PostFormArray("skills")
	if len(skills) == 0 {
		c.String(http.StatusBadRequest, "At least one skill is required")
		return
	}
	applicants, err := db.SearchApplicantsBySkills(skills)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to search applicants: "+err.Error())
		return
	}
	c.HTML(http.StatusOK, "search_applicants.html", gin.H{
		"Applicants": applicants,
	})
}

func handleLogout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Options(sessions.Options{
		Path:   "/",
		MaxAge: -1,
	})
	session.Save()
	c.Redirect(http.StatusFound, "/")
}
