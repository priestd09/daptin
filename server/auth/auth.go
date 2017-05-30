package auth

import (
  "net/http"
  "gopkg.in/gin-gonic/gin.v1"
  "github.com/gorilla/context"
  "github.com/auth0/go-jwt-middleware"
  "github.com/dgrijalva/jwt-go"
  log "github.com/Sirupsen/logrus"
  "github.com/jmoiron/sqlx"
  "github.com/artpar/api2go"
)

type CmsUser interface {
  GetName() string
  GetEmail() string
  IsGuest() bool
  IsLoggedIn() bool
}

type cmsUser struct {
  name       string
  email      string
  isLoggedIn bool
}

func (c *cmsUser) GetName() string {
  return c.name
}

func (c *cmsUser) GetEmail() string {
  return c.email
}

func (c *cmsUser) IsGuest() bool {
  return !c.isLoggedIn
}

func (c *cmsUser) IsLoggedIn() bool {
  return c.isLoggedIn
}

func GetUser(req *http.Request) *CmsUser {
  return nil
}

type AuthMiddleWare struct {
  db                *sqlx.DB
  userCrud          api2go.CRUD
  userGroupCrud     api2go.CRUD
  userUserGroupCrud api2go.CRUD
}

func NewAuthMiddlewareBuilder(db *sqlx.DB) *AuthMiddleWare {
  return &AuthMiddleWare{
    db: db,
  }
}

func (a *AuthMiddleWare) SetUserCrud(curd api2go.CRUD) {
  a.userCrud = curd
}

func (a *AuthMiddleWare) SetUserGroupCrud(curd api2go.CRUD) {
  a.userGroupCrud = curd
}

func (a *AuthMiddleWare) SetUserUserGroupCrud(curd api2go.CRUD) {
  a.userUserGroupCrud = curd
}

func NewAuthMiddleware(db *sqlx.DB, userCrud api2go.CRUD, userGroupCrud api2go.CRUD, userUserGroupCrud api2go.CRUD) *AuthMiddleWare {
  return &AuthMiddleWare{
    db:db,
    userCrud:userCrud,
    userGroupCrud:userGroupCrud,
    userUserGroupCrud:userUserGroupCrud,
  }
}

var jwtMiddleware = jwtmiddleware.New(jwtmiddleware.Options{
  ValidationKeyGetter: func(token *jwt.Token) (interface{}, error) {
    return []byte("nXhlfq1Q6llIOJgUBwGjx2knwRzJQVpSOYbnUmoZNwqBwAtH9IXfKmfbeEYcwFSc"), nil
  },
  //Debug: true,
  // When set, the middleware verifies that tokens are signed with the specific signing algorithm
  // If the signing method is not constant the ValidationKeyGetter callback can be used to implement additional checks
  // Important to avoid security issues described here: https://auth0.com/blog/2015/03/31/critical-vulnerabilities-in-json-web-token-libraries/
  SigningMethod: jwt.SigningMethodHS256,
  UserProperty: "user",
})

func StartsWith(bigStr string, smallString string) bool {
  if len(bigStr) < len(smallString) {
    return false
  }

  if bigStr[0:len(smallString)] == smallString {
    return true
  }

  return false

}

func (a *AuthMiddleWare) AuthCheckMiddleware(c *gin.Context) {

  if StartsWith(c.Request.RequestURI, "/static") || StartsWith(c.Request.RequestURI, "/favicon.ico") {
    c.Next()
    return
  }

  err := jwtMiddleware.CheckJWT(c.Writer, c.Request)
  log.Infof("Session user: %v", err)

  if err != nil {
    c.AbortWithError(401, err)
    return
  } else {

    user := context.Get(c.Request, "user")
    log.Infof("Set user: %v", user)
    if (user == nil) {
      context.Set(c.Request, "user_id", "")
      context.Set(c.Request, "usergroup_id", []string{})
      c.Next()
    } else {

      userToken := user.(*jwt.Token)
      email := userToken.Claims.(jwt.MapClaims)["email"].(string)
      //log.Infof("User is not nil: %v", email  )

      var referenceId string
      var userId int64
      var userGroups []string
      err := a.db.QueryRowx("select u.id, u.reference_id from user u where email = ?", email).Scan(&userId, &referenceId)

      if err != nil {
        log.Errorf("Failed to scan user from db: %v", err)

        mapData := make(map[string]interface{})
        mapData["name"] = email
        mapData["email"] = email

        newUser := api2go.NewApi2GoModelWithData("user", nil, 644, nil, mapData)

        req := api2go.Request{
          PlainRequest: &http.Request{
            Method: "POST",
          },
        }

        resp, err := a.userCrud.Create(newUser, req)
        if err != nil {
          log.Errorf("Failed to create new user: %v", err)
          c.AbortWithStatus(403)
          return
        }
        referenceId = resp.Result().(*api2go.Api2GoModel).Data["reference_id"].(string)

        mapData = make(map[string]interface{})
        mapData["name"] = "Home group for  user " + email

        newUserGroup := api2go.NewApi2GoModelWithData("usergroup", nil, 644, nil, mapData)

        resp, err = a.userGroupCrud.Create(newUserGroup, req)
        if err != nil {
          log.Errorf("Failed to create new user group: %v", err)
        }
        userGroupId := resp.Result().(*api2go.Api2GoModel).Data["reference_id"].(string)
        userGroups = []string{userGroupId}

        mapData = make(map[string]interface{})
        mapData["user_id"] = referenceId
        mapData["usergroup_id"] = userGroupId
        newUserUserGroup := api2go.NewApi2GoModelWithData("user_has_usergroup", nil, 644, nil, mapData)

        uug, err := a.userUserGroupCrud.Create(newUserUserGroup, req)
        log.Infof("Userug: %v", uug)

      } else {
        rows, err := a.db.Queryx("select ug.reference_id from usergroup ug join user_has_usergroup uug on uug.usergroup_id = ug.id where uug.user_id = ?", userId)
        if err != nil {

        } else {
          rows.Scan(userGroups)
          rows.Close()
        }
      }

      context.Set(c.Request, "user_id", referenceId)
      context.Set(c.Request, "user_id_integer", userId)
      context.Set(c.Request, "usergroup_id", userGroups)

      c.Next()

    }
  }

}