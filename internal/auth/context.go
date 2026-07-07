package auth

import "github.com/gin-gonic/gin"

const userContextKey = "auth.user"

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        Role   `json:"role"`
}

func SetUser(c *gin.Context, user User) {
	c.Set(userContextKey, user)
}

func CurrentUser(c *gin.Context) (User, bool) {
	value, ok := c.Get(userContextKey)
	if !ok {
		return User{}, false
	}

	user, ok := value.(User)
	return user, ok
}
