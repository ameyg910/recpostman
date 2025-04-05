package main

import (
    "encoding/json"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "rec_postman/db"
    "rec_postman/models"

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

    db.InitDB() // Initialize PostgreSQL
}

func main() {
    r := gin.Default()
    r.LoadHTMLGlob("templates/*")

    r.GET("/", func(c *gin.Context) {
        c.String(http.StatusOK, "Welcome to the Recruitment Portal!")
    })

    r.GET("/auth/google/login", handleGoogleLogin)
    r.GET("/auth/google/callback", handleGoogleCallback)
    r.GET("/select-role", handleSelectRole)
    r.POST("/select-role", handleRoleSubmission)

    r.Run(":8080")
}

func handleGoogleLogin(c *gin.Context) {
    url := googleOauthConfig.AuthCodeURL("state", oauth2.AccessTypeOffline)
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

    c.Redirect(http.StatusFound, "/select-role?id="+user.ID)
}

func handleSelectRole(c *gin.Context) {
    userID := c.Query("id")
    c.HTML(http.StatusOK, "select_role.html", gin.H{
        "UserID": userID,
    })
}

func handleRoleSubmission(c *gin.Context) {
    userID := c.PostForm("id")
    role := c.PostForm("role")

    user, err := db.GetUser(userID)
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

    c.String(http.StatusOK, "Role set to "+string(user.Role)+". Welcome, "+user.Name+"!")
}