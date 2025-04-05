package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"rec_postman/db"
	"rec_postman/models"

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

	log.Println("Google OAuth Config - ClientID:", googleOauthConfig.ClientID, "RedirectURL:", googleOauthConfig.RedirectURL)
	db.InitDB()
}

func main() {
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")

	store := cookie.NewStore([]byte("secret-key")) // Replace with a secure key in production
	r.Use(sessions.Sessions("mysession", store))

	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Welcome to the Recruitment Portal!")
	})
	r.GET("/auth/google/login", handleGoogleLogin)
	r.GET("/auth/google/callback", handleGoogleCallback)
	r.GET("/select-role", handleSelectRole)
	r.POST("/select-role", handleRoleSubmission)
	r.GET("/logout", handleLogout)

	r.GET("/dashboard", requireRole(models.SuperAdmin, models.Recruiter, models.Applicant), handleDashboard)
	r.GET("/admin", requireRole(models.SuperAdmin), func(c *gin.Context) {
		c.String(http.StatusOK, "Super Admin panel")
	})
	r.GET("/admin/approve-recruiters", requireRole(models.SuperAdmin), handleApproveRecruiters)
	r.POST("/admin/approve-recruiter", requireRole(models.SuperAdmin), handleApproveRecruiter)

	r.Run(":8080")
}

func handleGoogleLogin(c *gin.Context) {
	// Add prompt=select_account to force account selection
	url := googleOauthConfig.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "select_account"))
	log.Println("Redirecting to Google OAuth URL:", url)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func handleGoogleCallback(c *gin.Context) {
	code := c.Query("code")
	log.Println("Received authorization code:", code)
	token, err := googleOauthConfig.Exchange(c, code)
	if err != nil {
		log.Println("Token exchange failed:", err)
		c.String(http.StatusBadRequest, "Failed to exchange token: "+err.Error())
		return
	}

	client := googleOauthConfig.Client(c, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		log.Println("Failed to get user info:", err)
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
	case "super_admin", "recruiter", "applicant":
		user.Role = models.Role(role)
	default:
		c.String(http.StatusBadRequest, "Invalid role")
		return
	}

	if err := db.SaveUser(user); err != nil {
		c.String(http.StatusInternalServerError, "Failed to update role: "+err.Error())
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

	if user.Role == models.Recruiter {
		var approved bool
		db.DB.QueryRow("SELECT approved FROM users WHERE id = $1", user.ID).Scan(&approved)
		if !approved {
			c.String(http.StatusForbidden, "Your recruiter account is pending approval.")
			return
		}
	}

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"Name": user.Name,
		"Role": string(user.Role),
	})
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
	rows, err := db.DB.Query("SELECT id, email, name FROM users WHERE role = 'recruiter' AND approved = FALSE")
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch recruiters: "+err.Error())
		return
	}
	defer rows.Close()

	var recruiters []struct {
		ID    string
		Email string
		Name  string
	}
	for rows.Next() {
		var r struct {
			ID    string
			Email string
			Name  string
		}
		rows.Scan(&r.ID, &r.Email, &r.Name)
		recruiters = append(recruiters, r)
	}

	c.HTML(http.StatusOK, "approve_recruiters.html", gin.H{
		"Recruiters": recruiters,
	})
}

func handleApproveRecruiter(c *gin.Context) {
	userID := c.PostForm("id")
	_, err := db.DB.Exec("UPDATE users SET approved = TRUE WHERE id = $1 AND role = 'recruiter'", userID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to approve recruiter: "+err.Error())
		return
	}
	c.Redirect(http.StatusFound, "/admin/approve-recruiters")
}

func handleLogout(c *gin.Context) {
	session := sessions.Default(c)
	log.Println("Logging out user with ID:", session.Get("user_id"))
	session.Clear()                   // Clear session data
	session.Options(sessions.Options{ // Expire the cookie
		Path:   "/",
		MaxAge: -1,
	})
	if err := session.Save(); err != nil {
		log.Println("Failed to save session:", err)
		c.String(http.StatusInternalServerError, "Failed to log out: "+err.Error())
		return
	}
	log.Println("Session cleared, redirecting to /")
	// Redirect to Google logout to clear OAuth state (optional)
	c.Redirect(http.StatusFound, "/")
}
