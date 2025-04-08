package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"rec_postman/db"
	"rec_postman/models"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/ledongthuc/pdf"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	googleOauthConfig *oauth2.Config
	smtpAuth          smtp.Auth
	smtpAddr          = "smtp.gmail.com:587"
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
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
			"openid",
		},
		Endpoint: google.Endpoint,
	}

	if googleOauthConfig.ClientID == "" || googleOauthConfig.ClientSecret == "" {
		log.Fatal("GOOGLE_CLIENT_ID or GOOGLE_CLIENT_SECRET not set in .env")
	}

	smtpAuth = smtp.PlainAuth("", os.Getenv("SMTP_EMAIL"), os.Getenv("SMTP_APP_PASSWORD"), "smtp.gmail.com")
	if os.Getenv("SMTP_EMAIL") == "" || os.Getenv("SMTP_APP_PASSWORD") == "" {
		log.Fatal("SMTP_EMAIL or SMTP_APP_PASSWORD not set in .env")
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
		session := sessions.Default(c)
		userID := session.Get("user_id")
		var name string
		if userID != nil {
			user, err := db.GetUser(userID.(string))
			if err != nil {
				log.Println("Failed to fetch user for home page:", err)
				c.String(http.StatusInternalServerError, "Error fetching user")
				return
			}
			name = user.Name
		}
		c.HTML(http.StatusOK, "index.html", gin.H{
			"UserID": userID,
			"Name":   name,
		})
	})
	r.GET("/auth/google/login", handleGoogleLogin)
	r.GET("/auth/google/callback", handleGoogleCallback)
	r.GET("/select-role", handleSelectRole)
	r.POST("/select-role", handleRoleSubmission)
	r.GET("/logout", handleLogout)
	r.POST("/applicant/follow-company", requireRole(models.Applicant), handleFollowCompany)

	r.GET("/dashboard", requireRole(models.SuperAdmin, models.Recruiter, models.Applicant), handleDashboard)
	r.POST("/recruiter/post-job", requireRole(models.Recruiter), handlePostJob)
	r.GET("/recruiter/search-applicants", requireRole(models.Recruiter), handleSearchApplicantsForm)
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
	session := sessions.Default(c)
	userID := session.Get("user_id")
	log.Println("handleGoogleLogin: userID from session:", userID)
	if userID != nil {
		log.Println("User is logged in, redirecting to /dashboard")
		c.Redirect(http.StatusFound, "/dashboard")
		c.Abort()
		return
	}
	log.Println("User not logged in, proceeding with OAuth")
	url := googleOauthConfig.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "select_account"))
	log.Println("Redirecting to Google OAuth URL:", url)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func handleGoogleCallback(c *gin.Context) {
	log.Println("Entering handleGoogleCallback")
	code := c.Query("code")
	log.Println("Received code:", code)
	if code == "" {
		log.Println("No code provided in callback")
		c.String(http.StatusBadRequest, "No code provided")
		return
	}

	token, err := googleOauthConfig.Exchange(c, code)
	if err != nil {
		log.Println("Failed to exchange token:", err)
		c.String(http.StatusBadRequest, "Failed to exchange token: "+err.Error())
		return
	}
	log.Println("Token received:", token.AccessToken)

	client := googleOauthConfig.Client(c, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		log.Println("Failed to get user info:", err)
		c.String(http.StatusBadRequest, "Failed to get user info: "+err.Error())
		return
	}
	defer resp.Body.Close()

	log.Println("Response Status:", resp.Status)
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("Failed to read response:", err)
		c.String(http.StatusInternalServerError, "Failed to read response: "+err.Error())
		return
	}
	log.Println("User info response:", string(data))

	var userInfo struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"given_name"`
	}
	if err := json.Unmarshal(data, &userInfo); err != nil {
		log.Println("Failed to parse user info:", err)
		c.String(http.StatusInternalServerError, "Failed to parse user info: "+err.Error())
		return
	}
	log.Println("User info parsed:", userInfo.ID, userInfo.Email)

	user := models.User{
		ID:    userInfo.ID,
		Email: userInfo.Email,
		Name:  userInfo.Name,
	}

	existingUser, err := db.GetUser(user.ID)
	if err != nil {
		log.Println("Failed to check existing user:", err)
		c.String(http.StatusInternalServerError, "Failed to check user: "+err.Error())
		return
	}

	session := sessions.Default(c)
	if existingUser != nil && existingUser.Role != "" {
		log.Println("Existing user with role found:", existingUser.Role)
		session.Set("user_id", user.ID)
		session.Save()
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}

	if user.Email == "ameygupta880@gmail.com" {
		log.Println("Assigning SuperAdmin to ameygupta880@gmail.com")
		user.Role = models.SuperAdmin
		if err := db.SaveUser(&user); err != nil {
			log.Println("Failed to save super admin:", err)
			c.String(http.StatusInternalServerError, "Failed to save super admin: "+err.Error())
			return
		}
		session.Set("user_id", user.ID)
		session.Save()
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}

	if err := db.SaveUser(&user); err != nil {
		log.Println("Failed to save user:", err)
		c.String(http.StatusInternalServerError, "Failed to save user: "+err.Error())
		return
	}

	session.Set("user_id", user.ID)
	session.Save()
	log.Println("Redirecting to /select-role for new user")
	c.Redirect(http.StatusFound, "/select-role?id="+user.ID)
}

func handleSelectRole(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id")
	log.Println("handleSelectRole: userID from session:", userID)
	if userID == nil {
		log.Println("No session, redirecting to /auth/google/login")
		c.Redirect(http.StatusFound, "/auth/google/login")
		return
	}

	user, err := db.GetUser(userID.(string))
	if err != nil {
		log.Println("Failed to fetch user:", err)
		c.String(http.StatusInternalServerError, "Failed to fetch user: "+err.Error())
		return
	}
	log.Println("User role:", user.Role)

	if user.Role != "" {
		log.Println("Role already set, redirecting to /dashboard")
		c.Redirect(http.StatusFound, "/dashboard")
		c.Abort()
		return
	}

	c.HTML(http.StatusOK, "select_role.html", gin.H{
		"UserID": userID,
		"Email":  user.Email,
	})
}

func handleRoleSubmission(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id")
	log.Println("handleRoleSubmission: userID from session:", userID)
	if userID == nil {
		log.Println("No session, redirecting to /auth/google/login")
		c.Redirect(http.StatusFound, "/auth/google/login")
		return
	}

	user, err := db.GetUser(userID.(string))
	if err != nil {
		log.Println("Failed to fetch user:", err)
		c.String(http.StatusBadRequest, "User not found: "+err.Error())
		return
	}

	if user.Role != "" {
		log.Println("Role already assigned:", user.Role)
		c.HTML(http.StatusForbidden, "error.html", gin.H{"Message": "Role already assigned and cannot be changed"})
		return
	}

	role := c.PostForm("role")
	log.Println("Selected role:", role)
	if user.Email == "ameygupta880@gmail.com" {
		log.Println("Attempt to change SuperAdmin role blocked")
		c.HTML(http.StatusForbidden, "error.html", gin.H{"Message": "This account is reserved for Super Admin only"})
		return
	}

	switch role {
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
			log.Println("Failed to save company:", err)
			c.String(http.StatusInternalServerError, "Failed to save company: "+err.Error())
			return
		}
		user.CompanyID = strconv.Itoa(companyID)
	case "applicant":
		user.Role = models.Applicant
		skills := c.PostFormArray("skills")
		if len(skills) == 0 {
			c.HTML(http.StatusBadRequest, "select_role.html", gin.H{
				"UserID":  userID,
				"Email":   user.Email,
				"Message": "At least one skill is required for applicants",
			})
			return
		}
		user.Skills = skills
	default:
		log.Println("Invalid role selected:", role)
		c.String(http.StatusBadRequest, "Invalid role")
		return
	}

	if err := db.SaveUser(user); err != nil {
		log.Println("Failed to update user:", err)
		c.String(http.StatusInternalServerError, "Failed to update user: "+err.Error())
		return
	}
	log.Println("Role assigned successfully:", user.Role)
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
		err := db.DB.QueryRow("SELECT approved FROM users WHERE id = $1", user.ID).Scan(&approved)
		if err != nil {
			log.Println("Failed to check approval status:", err)
			c.String(http.StatusInternalServerError, "Failed to check approval status: "+err.Error())
			return
		}
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
		applications, err := db.GetApplicationsByRecruiter(user.ID)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to fetch applications: "+err.Error())
			return
		}
		c.HTML(http.StatusOK, "recruiter_dashboard.html", gin.H{
			"Name":         user.Name,
			"Company":      company,
			"Jobs":         jobs,
			"Applications": applications,
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
		followedCompanies, err := db.GetFollowedCompanies(user.ID)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to fetch followed companies: "+err.Error())
			return
		}
		c.HTML(http.StatusOK, "applicant_dashboard.html", gin.H{
			"Name":              user.Name,
			"Skills":            user.Skills,
			"Jobs":              jobs,
			"Applications":      applications,
			"RecommendedJobs":   recommendedJobs,
			"Interviews":        interviews,
			"Resume":            user.Resume,
			"FollowedCompanies": followedCompanies,
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

	followers, err := db.GetCompanyFollowers(companyID)
	if err != nil {
		log.Println("Failed to fetch followers for notification:", err)
	} else {
		for _, follower := range followers {
			go sendJobNotification(follower.Email, title, companyID)
		}
	}

	c.Redirect(http.StatusFound, "/dashboard")
}

func handleSearchApplicantsForm(c *gin.Context) {
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

	c.HTML(http.StatusOK, "search_applicants.html", gin.H{
		"Name": user.Name,
	})
}

func handleSearchApplicants(c *gin.Context) {
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

	skills := c.PostFormArray("skills")
	if len(skills) == 0 {
		c.HTML(http.StatusBadRequest, "search_applicants.html", gin.H{
			"Name":    user.Name,
			"Message": "At least one skill is required",
		})
		return
	}
	applicants, err := db.SearchApplicantsBySkills(skills)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to search applicants: "+err.Error())
		return
	}
	c.HTML(http.StatusOK, "search_applicants.html", gin.H{
		"Name":             user.Name,
		"Applicants":       applicants,
		"ApplicantsLoaded": true,
	})
}

func handleApplyJob(c *gin.Context) {
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
	if user.Resume == "" || len(user.Skills) == 0 {
		jobs, _ := db.GetAllJobs()
		c.HTML(http.StatusBadRequest, "applicant_dashboard.html", gin.H{
			"Name":    user.Name,
			"Jobs":    jobs,
			"Skills":  user.Skills,
			"Resume":  user.Resume,
			"Message": "Please upload a resume and add skills before applying.",
		})
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
		Resume:      user.Resume,
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

	// Validate file extension
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".pdf") {
		log.Println("Invalid file type, must be PDF")
		renderApplicantDashboard(c, userID.(string), "Please upload a PDF file.")
		return
	}

	// Ensure upload directory exists
	uploadDir := "./uploads"
	if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
		log.Println("Creating upload directory")
		if err := os.Mkdir(uploadDir, 0755); err != nil {
			log.Println("Failed to create upload directory:", err)
			c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to create upload directory: " + err.Error()})
			return
		}
	}

	// Save and validate PDF
	tempFilePath := filepath.Join(uploadDir, "temp_"+file.Filename)
	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		log.Println("Failed to save temp file:", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to save resume: " + err.Error()})
		return
	}
	defer os.Remove(tempFilePath)

	valid, errMsg := validatePDF(tempFilePath)
	if !valid {
		log.Println("PDF validation failed:", errMsg)
		renderApplicantDashboard(c, userID.(string), errMsg)
		return
	}

	filename := userID.(string) + "_" + file.Filename
	filePath := filepath.Join(uploadDir, filename)
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		log.Println("Failed to save file:", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to save resume: " + err.Error()})
		return
	}
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Println("File does not exist after saving:", filePath)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "File save verification failed"})
		return
	}
	log.Println("File saved successfully to:", filePath)

	user, err := db.GetUser(userID.(string))
	if err != nil {
		log.Println("Failed to fetch user:", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to fetch user: " + err.Error()})
		return
	}
	user.Resume = "/uploads/" + filename
	if err := db.SaveUser(user); err != nil {
		log.Println("Failed to update user:", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to update user with resume: " + err.Error()})
		return
	}
	c.Redirect(http.StatusFound, "/dashboard")
}

func validatePDF(filePath string) (bool, string) {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return false, "Failed to open PDF: " + err.Error()
	}
	defer f.Close()

	totalPage := r.NumPage()
	var text string
	for i := 1; i <= totalPage; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		pageText, err := p.GetPlainText(nil)
		if err != nil {
			return false, "Failed to extract text from page " + strconv.Itoa(i) + ": " + err.Error()
		}
		text += pageText
	}
	log.Println("Extracted PDF text:", text) // Debug log

	text = strings.ToLower(text)
	// Broaden "name" detection
	hasName := strings.Contains(text, "name") ||
		strings.Contains(text, "resume of") ||
		len(text) > 50 // Assume a long text likely includes a name
	hasSkills := strings.Contains(text, "skills") || strings.Contains(text, "skill")
	hasEducation := strings.Contains(text, "education") ||
		strings.Contains(text, "degree") ||
		strings.Contains(text, "university") ||
		strings.Contains(text, "college")

	if !hasName || !hasSkills || !hasEducation {
		missing := []string{}
		if !hasName {
			missing = append(missing, "name")
		}
		if !hasSkills {
			missing = append(missing, "skills")
		}
		if !hasEducation {
			missing = append(missing, "education")
		}
		return false, "Incomplete resume: missing " + strings.Join(missing, ", ") + "."
	}
	return true, ""
}

func renderApplicantDashboard(c *gin.Context, userID string, message string) {
	user, err := db.GetUser(userID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to fetch user: " + err.Error()})
		return
	}
	jobs, err := db.GetAllJobs()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to fetch jobs: " + err.Error()})
		return
	}
	applications, err := db.GetApplicationsByApplicant(userID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to fetch applications: " + err.Error()})
		return
	}
	recommendedJobs, err := db.GetRecommendedJobs(user.Skills)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to fetch recommended jobs: " + err.Error()})
		return
	}
	interviews, err := db.GetInterviewsByApplicant(userID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to fetch interviews: " + err.Error()})
		return
	}
	followedCompanies, err := db.GetFollowedCompanies(userID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": "Failed to fetch followed companies: " + err.Error()})
		return
	}
	c.HTML(http.StatusBadRequest, "applicant_dashboard.html", gin.H{
		"Name":              user.Name,
		"Skills":            user.Skills,
		"Jobs":              jobs,
		"Applications":      applications,
		"RecommendedJobs":   recommendedJobs,
		"Interviews":        interviews,
		"Resume":            user.Resume,
		"FollowedCompanies": followedCompanies,
		"Message":           message,
	})
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
	scheduledAtStr := c.PostForm("scheduled_at")

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

	job, err := db.GetJob(jobID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch job: "+err.Error())
		return
	}
	applicant, err := db.GetUser(applicantID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch applicant: "+err.Error())
		return
	}

	interview := models.Interview{
		JobID:       jobID,
		ApplicantID: applicantID,
		RecruiterID: userID.(string),
		Status:      "requested",
		ScheduledAt: scheduledAt,
	}

	interviewID, err := db.SaveInterview(&interview)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to request interview: "+err.Error())
		return
	}

	go sendInterviewNotification(applicant.Email, job.Title, scheduledAt, interviewID)

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
	status := c.PostForm("status")
	alternativeTimeStr := c.PostForm("alternative_time")

	interviewID, err := strconv.Atoi(interviewIDStr)
	if err != nil || (status != "accepted" && status != "declined") {
		c.String(http.StatusBadRequest, "Invalid interview ID or status")
		return
	}

	var alternativeTime time.Time
	if status == "declined" && alternativeTimeStr != "" {
		alternativeTime, err = time.Parse("2006-01-02 15:04", alternativeTimeStr)
		if err != nil {
			c.String(http.StatusBadRequest, "Invalid alternative time format (use YYYY-MM-DD HH:MM)")
			return
		}
	}

	interview, err := db.GetInterview(interviewID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch interview: "+err.Error())
		return
	}

	if err := db.UpdateInterviewStatus(interviewID, status, alternativeTime); err != nil {
		c.String(http.StatusInternalServerError, "Failed to update interview: "+err.Error())
		return
	}

	if status == "declined" && !alternativeTime.IsZero() {
		recruiter, err := db.GetUser(interview.RecruiterID)
		if err != nil {
			log.Println("Failed to fetch recruiter for notification:", err)
		} else {
			job, err := db.GetJob(interview.JobID)
			if err != nil {
				log.Println("Failed to fetch job for notification:", err)
			} else {
				go sendAlternativeTimeNotification(recruiter.Email, job.Title, userID.(string), alternativeTime)
			}
		}
	}

	c.Redirect(http.StatusFound, "/dashboard")
}

func handleFollowCompany(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id")
	if userID == nil {
		c.Redirect(http.StatusFound, "/auth/google/login")
		return
	}

	companyIDStr := c.PostForm("company_id")
	companyID, err := strconv.Atoi(companyIDStr)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid company ID")
		return
	}

	if err := db.FollowCompany(userID.(string), companyID); err != nil {
		c.String(http.StatusInternalServerError, "Failed to follow company: "+err.Error())
		return
	}

	c.Redirect(http.StatusFound, "/dashboard")
}

func sendInterviewNotification(toEmail, jobTitle string, scheduledAt time.Time, interviewID int) {
	msg := []byte(fmt.Sprintf(
		"Subject: Interview Scheduled for %s\r\n"+
			"\r\n"+
			"Dear Applicant,\r\n"+
			"You have been invited to an interview for the position of %s.\r\n"+
			"Date & Time: %s\r\n"+
			"Please respond by accepting or declining this interview at: http://localhost:8080/dashboard\r\n"+
			"Interview ID: %d\r\n"+
			"\r\n"+
			"Best regards,\r\n"+
			"Recruitment Team\r\n",
		jobTitle, jobTitle, scheduledAt.Format("2006-01-02 15:04"), interviewID))

	err := smtp.SendMail(smtpAddr, smtpAuth, os.Getenv("SMTP_EMAIL"), []string{toEmail}, msg)
	if err != nil {
		log.Println("Failed to send interview notification:", err)
	} else {
		log.Println("Interview notification sent to:", toEmail)
	}
}

func sendAlternativeTimeNotification(toEmail, jobTitle, applicantID string, alternativeTime time.Time) {
	msg := []byte(fmt.Sprintf(
		"Subject: Applicant Suggested New Time for %s\r\n"+
			"\r\n"+
			"Dear Recruiter,\r\n"+
			"The applicant (ID: %s) has declined the interview for %s and suggested a new time:\r\n"+
			"New Date & Time: %s\r\n"+
			"Please review and reschedule at: http://localhost:8080/dashboard\r\n"+
			"\r\n"+
			"Best regards,\r\n"+
			"Recruitment Team\r\n",
		jobTitle, applicantID, jobTitle, alternativeTime.Format("2006-01-02 15:04")))

	err := smtp.SendMail(smtpAddr, smtpAuth, os.Getenv("SMTP_EMAIL"), []string{toEmail}, msg)
	if err != nil {
		log.Println("Failed to send alternative time notification:", err)
	} else {
		log.Println("Alternative time notification sent to:", toEmail)
	}
}

func sendJobNotification(toEmail, jobTitle string, companyID int) {
	msg := []byte(fmt.Sprintf(
		"Subject: New Job Posting: %s\r\n"+
			"\r\n"+
			"Dear Applicant,\r\n"+
			"A new job '%s' has been posted by a company you follow (ID: %d).\r\n"+
			"View it at: http://localhost:8080/dashboard\r\n"+
			"\r\n"+
			"Best regards,\r\n"+
			"Recruitment Team\r\n",
		jobTitle, jobTitle, companyID))

	err := smtp.SendMail(smtpAddr, smtpAuth, os.Getenv("SMTP_EMAIL"), []string{toEmail}, msg)
	if err != nil {
		log.Println("Failed to send job notification:", err)
	} else {
		log.Println("Job notification sent to:", toEmail)
	}
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
