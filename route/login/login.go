package login

import (
  "context"
  "encoding/json"
  "fmt"
  "io/ioutil"
  "micrified.com/internal/user"
  "micrified.com/route"
  "micrified.com/service/auth"
  "net/http"
  "time"
)


// Data: Login
type loginData struct {
  TimeFormat, UserTable, CredentialTable string
}

// Controller: Login
type Controller route.ControllerType[loginData]


/*\
 *******************************************************************************
 *                            Interface: Controller                            *
 *******************************************************************************
\*/


func NewController (s route.Service) Controller {
  return Controller {
    Name:             "login",
    Methods: map[string]route.Method {
      http.MethodPost: route.Restful.Post,
    },
    Service:           s,
    Limit:             5 * time.Second,
    Data: loginData {
      TimeFormat:      "2006-01-02 15:04:05",
      UserTable:       "users",
      CredentialTable: "credentials",
    },
  }
}

func (c *Controller) Route () string {
  return "/" + c.Name
}

func (c *Controller) Handler (s string) route.Method {
  if method, ok := c.Methods[s]; ok {
    return method
  }
  return nil
}

func (c *Controller) Timeout () time.Duration {
  return c.Limit
}


/*\
 *******************************************************************************
 *                             Interface: Restful                              *
 *******************************************************************************
\*/


func (c *Controller) Get (x context.Context, rq *http.Request, re *route.Result) error {
  return re.Unimplemented()
}

type LoginCredential struct {
  Username        string `json:"userid"`
  Passphrase      string `json:"passphrase"`
  Period          string `json:"period"`
}

type StoredCredential struct {
  Hash, Salt []byte
}

type SessionCredential struct {
  Secret     string `json:"secret"`
  Expiration string `json:"expiration"`
}

func (c *Controller) Post (x context.Context, rq *http.Request, re *route.Result) error {
  var (
    body    []byte              = []byte{}
    err     error               = nil
    ip      string              = x.Value(user.UserIPKey).(string)
    login   LoginCredential     = LoginCredential{}
  )

  fail := func (err error, status int) error {
    re.Status = status
    return err
  }

  // Read request body
  if body, err = ioutil.ReadAll(rq.Body); nil != err {
    return fail(err, http.StatusInternalServerError)
  }

  // Unmarshal to type
  if err = json.Unmarshal(body, &login); nil != err {
    return fail(err, http.StatusBadRequest)
  }

  // Check if a retry penalty exists (IP must exist)
  if c.Service.Auth.Penalised(ip) {
    return fail(fmt.Errorf("Try again later"), http.StatusTooManyRequests)
  }

  // Extract stored login credentials
  q := fmt.Sprintf("SELECT b.hash, b.salt " +
                   "FROM %s AS a INNER JOIN %s AS b " +
		   "ON a.id = b.user_id " +
		   "WHERE a.username = ?", 
		   c.Data.UserTable, c.Data.CredentialTable)

  // Define the authentication routine
  doAuth := func () (bool, error) {
    var stored StoredCredential

    rows, err := c.Service.Database.DB.Query(q, login.Username)
    if nil != err {
      return false, err
    }
    if !rows.Next() { // No error implies non-infrastructure related error
      fmt.Println("No account")
      return false, nil
    }
    if err = rows.Scan(&stored.Hash, &stored.Salt); nil != err {
      return false, err
    }
    fmt.Println("Comparing credentials ...")
    return auth.Compare(login.Passphrase, stored.Salt, stored.Hash), nil
  }

  // Perform authentication 
  session, ok, err := c.Service.Auth.Authenticate(ip, login.Username, 
    login.Period, doAuth)
  if err != nil {
    // TODO: Don't leak info here
    return fail(err, http.StatusInternalServerError)
  }

  // Wipe penalty and create session if OK; else penalise and return error
  if ok {
    c.Service.Auth.NoPenalty(ip)
  } else {
    c.Service.Auth.Penalise(ip)
    return fail(fmt.Errorf("Bad credentials"), http.StatusUnauthorized)
  }

  // Compose response
  fmt.Printf("Response should be OK: %+v\n", re)
  return re.Marshal(route.ContentTypeJSON,
    &SessionCredential {
      Secret:      session.Secret.HexString(),
      Expiration:  session.Expiration.Format(c.Data.TimeFormat),
  })
}

func (c *Controller) Put (x context.Context, rq *http.Request, re *route.Result) error {
  return re.Unimplemented()
}

func (c *Controller) Delete (x context.Context, rq *http.Request, re *route.Result) error {
  return re.Unimplemented()
}

