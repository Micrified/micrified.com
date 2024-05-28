// Package blog implements a RESTful endpoint for blogs.
// It supports the creation, modification, and deletion of blog posts
package blog

import (
  "context"
  "database/sql"
  "encoding/json"
  "fmt"
  "io/ioutil"
  "micrified.com/internal/user"
  "micrified.com/route"
  "micrified.com/service/auth"
  "net/http"
  "strconv"
  "time"
)


/*\
 *******************************************************************************
 *                                  Constants                                  *
 *******************************************************************************
\*/


const (
  RouteName string     = "blog"
  RouteListName string = "blogs"
)


/*\
 *******************************************************************************
 *                              Type Definitions                               *
 *******************************************************************************
\*/


// Data: Blog
type blogDataType struct {
  TimeFormat, IndexTable, ContentTable string
}

// Controller: Blog
type Controller route.ControllerType[blogDataType]

// ListController: Blog
type ListController route.ControllerType[blogDataType]


/*\
 *******************************************************************************
 *                              Global Variables                               *
 *******************************************************************************
\*/


var blogData blogDataType = blogDataType {
  TimeFormat:   "2006-01-02 15:04:05",
  IndexTable:   "blog_pages",
  ContentTable: "page_content",
}


/*\
 *******************************************************************************
 *                                Constructors                                 *
 *******************************************************************************
\*/


func NewListController (s route.Service) ListController {
  return ListController {
    Name: RouteListName,
    Methods: map[string]route.Method {
      http.MethodGet: route.Restful.Get,
    },
    Service: s,
    Limit:   5 * time.Second,
    Data:    blogData,
  }
}

func NewController (s route.Service) Controller {
  return Controller {
    Name:                RouteName,
    Methods: map[string]route.Method {
      http.MethodGet:    route.Restful.Get,
      http.MethodPost:   route.Restful.Post,
      http.MethodPut:    route.Restful.Put,
      http.MethodDelete: route.Restful.Delete,
    },
    Service:             s,
    Limit:               5 * time.Second,
    Data:                blogData,
  }
}


/*\
 *******************************************************************************
 *                            Interface: Controller                            *
 *******************************************************************************
\*/


// Controller

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


// ListController

func (c *ListController) Route () string {
  return "/" + c.Name
}

func (c *ListController) Handler (s string) route.Method {
  if method, ok := c.Methods[s]; ok {
    return method
  }
  return nil
}

func (c *ListController) Timeout () time.Duration {
  return c.Limit
}


/*\
 *******************************************************************************
 *                             Interface: Restful                              *
 *******************************************************************************
\*/


// Controller

type BlogResponse struct {
  ID       string `json:"id"`
  Title    string `json:"title"`
  Subtitle string `json:"subtitle"`
  Tag      string `json:"tag"`
  Body     string `json:"body"`
  Created  string `json:"created"`
  Updated  string `json:"updated"`
}

func (c *Controller) Get (x context.Context, rq *http.Request, re *route.Result) error {
  var (
    blog    BlogResponse = BlogResponse{}
    blog_id int          = -1
    err     error        = nil
  )

  fail := func(err error, status int) error {
    re.Status = status
    return err
  }

  q := fmt.Sprintf("SELECT a.id, a.title, a.subtitle, a.tag, b.body, b.created, b.updated " + 
                   "FROM %s AS a INNER JOIN %s AS b " +
		   "ON a.content_id = b.id " + 
		   "WHERE a.id = ?", c.Data.IndexTable, c.Data.ContentTable)

  // Validate ID
  if blog_id, err = strconv.Atoi(rq.URL.Query().Get("id")); nil != err {
    return fail(fmt.Errorf("Invalid query parameter"), http.StatusBadRequest)
  }

  // Extract row
  rows, err := c.Service.Database.DB.Query(q, blog_id)
  if nil != err {
    return fail(err, http.StatusInternalServerError)
  }
  defer rows.Close()

  // Verify entry exists
  if !rows.Next() {
    return fail(fmt.Errorf("Blog %s not found", blog_id), http.StatusNotFound)
  }

  // Marshal rows
  if err = rows.Scan(&blog.ID, &blog.Title, &blog.Subtitle, &blog.Tag,
    &blog.Body, &blog.Created, &blog.Updated); nil != err {
    return fail(err, http.StatusInternalServerError)
  }

  // Write to buffer and return any encoding error
  return re.Marshal(route.ContentTypeJSON, &blog)
}

type BlogPost struct {
  Title    string `json:"title"`
  Subtitle string `json:"subtitle"`
  Tag      string `json:"tag"`
  Body     string `json:"body"`
}

type BlogPostResponse struct {
  ID       string `json:"id"`
  Title    string `json:"title"`
  Subtitle string `json:"subtitle"`
  Tag      string `json:"tag"`
  Body     string `json:"body"`
  Created  string `json:"created"`
  Updated  string `json:"updated"`
}

func (c *Controller) Post (x context.Context, rq *http.Request, re *route.Result) error {
  var (
    body      []byte                  = []byte{}
    err       error                   = nil
    ip        string                  = x.Value(user.UserIPKey).(string)
    post      auth.AuthData[BlogPost] = auth.AuthData[BlogPost]{}
    timeStamp time.Time               = time.Now().UTC()
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
  if err = json.Unmarshal(body, &post); nil != err {
    return fail(err, http.StatusBadRequest)
  }

  // Check if authorized
  if err = c.Service.Auth.Authorized(ip, post.Username, post.Secret); nil != err {
    return fail(err, http.StatusUnauthorized)
  }
    
  // Define insert content
  insertBody := func (lastResult sql.Result, t *sql.Tx) (sql.Result, error) {
    q := fmt.Sprintf("INSERT INTO %s (created,updated,body) VALUES (?,?,?)",
      c.Data.ContentTable)
    return t.ExecContext(c.Service.Database.Context, q, timeStamp, timeStamp,
      post.Data.Body)
  }

  // Define insert record
  insertRecord := func (lastResult sql.Result, t *sql.Tx) (sql.Result, error) {
    id, err := lastResult.LastInsertId()
    if nil != err {
      return nil, err
    }
    q := fmt.Sprintf("INSERT INTO %s (title,subtitle,tag,content_id) " +
      "VALUES (?,?,?,?)", c.Data.IndexTable)
    return t.ExecContext(c.Service.Database.Context, q, post.Data.Title, 
      post.Data.Subtitle, post.Data.Tag, id)
  }

  // Execute sequenced insert operations; get back result
  r, err := c.Service.Database.Transaction(insertBody, insertRecord)
  if nil != err {
    return fail(err, http.StatusInternalServerError)
  }

  // Get the record ID
  id, err := r.LastInsertId()
  if nil != err {
    return fail(err, http.StatusInternalServerError)
  }

  // Write to buffer and return any encoding error
  return re.Marshal(route.ContentTypeJSON, 
    &BlogPostResponse {
      ID:       strconv.FormatInt(id, 10),
      Title:    post.Data.Title,
      Subtitle: post.Data.Subtitle,
      Tag:      post.Data.Tag,
      Body:     post.Data.Body,
      Created:  timeStamp.Format(c.Data.TimeFormat),
      Updated:  timeStamp.Format(c.Data.TimeFormat),
    })
}

type BlogPut struct {
  ID       string `json:"id"`
  Title    string `json:"title"`
  Subtitle string `json:"subtitle"`
  Tag      string `json:"tag"`
  Body     string `json:"body"`
}

type BlogPutResponse struct {
  ID       string `json:"id"`
  Title    string `json:"title"`
  Subtitle string `json:"subtitle"`
  Tag      string `json:"tag"`
  Updated  string `json:"updated"`
  Body     string `json:"body"`
}

func (c *Controller) Put (x context.Context, rq *http.Request, re *route.Result) error {
  var (
    body      []byte                 = []byte{}
    err       error                  = nil
    ip        string                 = x.Value(user.UserIPKey).(string)
    post      auth.AuthData[BlogPut] = auth.AuthData[BlogPut]{}
    timeStamp time.Time              = time.Now().UTC()
  )

  fail := func (err error, status int) error {
    re.Status = status
    return err
  }

  // Define update record
  updateRecord := func (lastResult sql.Result, conn *sql.Conn) (sql.Result, error) {
    q := fmt.Sprintf("UPDATE %s AS a INNER JOIN %s AS b ON a.content_id = b.id " +
                     "SET a.title = ?, a.subtitle = ?, b.updated = ?, b.body = ? " +
		     "WHERE a.id = ?", c.Data.IndexTable, c.Data.ContentTable)
    return conn.ExecContext(c.Service.Database.Context, q, post.Data.Title, 
      post.Data.Subtitle, timeStamp, post.Data.Body, post.Data.ID)
  }

  // Read request body
  if body, err = ioutil.ReadAll(rq.Body); nil != err {
    return fail(err, http.StatusInternalServerError)
  }

  // Unmarshal to type
  if err = json.Unmarshal(body, &post); nil != err {
    return fail(err, http.StatusBadRequest)
  }

  // Check if authorized
  if err = c.Service.Auth.Authorized(ip, post.Username, post.Secret); nil != err {
    return fail(err, http.StatusUnauthorized)
  }

  // Execute sequenced connection operations; get back result
  r, err := c.Service.Database.Connection(updateRecord)
  if nil != err {
    return fail(err, http.StatusInternalServerError)
  }

  // Verify the right number of rows were affected
  n, err := r.RowsAffected()
  if nil != err {
    return fail(err, http.StatusInternalServerError)
  } else if 0 == n {
    return fail(fmt.Errorf("Unexpected database result (no rows modified)"), 
      http.StatusInternalServerError)
  }

  // No difference is needed here in the return type
  return re.Marshal(route.ContentTypeJSON,
    &BlogPutResponse {
      ID:       post.Data.ID,
      Title:    post.Data.Title,
      Subtitle: post.Data.Subtitle,
      Tag:      post.Data.Tag,
      Updated:  timeStamp.Format(c.Data.TimeFormat),
      Body:     post.Data.Body,
    })
}

type BlogDelete struct {
  ID string `json:"id"`
}

func (c *Controller) Delete (x context.Context, rq *http.Request, re *route.Result) error {
  var (
    body  []byte                    = []byte{}
    err   error                     = nil
    ip    string                    = x.Value(user.UserIPKey).(string)
    post  auth.AuthData[BlogDelete] = auth.AuthData[BlogDelete]{}
  )

  fail := func (err error, status int) error {
    re.Status = status
    return err
  }

  // Define delete record
  deleteRecord := func (lastResult sql.Result, conn *sql.Conn) (sql.Result, error) {
    q := fmt.Sprintf("DELETE a, b FROM %s AS a INNER JOIN %s AS b " +
                     "ON a.content_id = b.id " +
                     "WHERE a.id = ?", c.Data.IndexTable, c.Data.ContentTable)
    return conn.ExecContext(c.Service.Database.Context, q, post.Data.ID)
  }

  // Read request body
  if body, err = ioutil.ReadAll(rq.Body); nil != err {
    return fail(err, http.StatusInternalServerError)
  }

  // Unmarshal to type
  if err = json.Unmarshal(body, &post); nil != err {
    return fail(err, http.StatusBadRequest)
  }

  // Check if authorized
  if err = c.Service.Auth.Authorized(ip, post.Username, post.Secret); nil != err {
    return fail(err, http.StatusUnauthorized)
  }


  // Execute sequenced connection operations; get back result
  r, err := c.Service.Database.Connection(deleteRecord)
  if nil != err {
    fail(err, http.StatusInternalServerError)
  }

  // Verify the right number of rows were affected
  n, err := r.RowsAffected()
  if nil != err {
    fail(err, http.StatusInternalServerError)
  } else if 2 != n {
    fail(fmt.Errorf("Unexpected database result (expected %d rows affected, got %d)",
      2, n), http.StatusInternalServerError)
  }

  return nil
}


// ListController

type BlogHeader struct {
  ID       string `json:"id"`
  Title    string `json:"title"`
  Subtitle string `json:"subtitle"`
  Tag      string `json:"tag"`
  Created  string `json:"created"`
  Updated  string `json:"updated"`
}

func (c *ListController) Get (x context.Context, rq *http.Request, re *route.Result) error {
  var (
    head BlogHeader
    list []BlogHeader
  )
  q := fmt.Sprintf("SELECT a.id, a.title, a.subtitle, a.tag, b.created, b.updated " +
                   "FROM %s AS a INNER JOIN %s AS b " + 
                   "ON a.content_id = b.id " +
                   "ORDER BY b.created", c.Data.IndexTable, c.Data.ContentTable)

  // Extract rows
  rows, err := c.Service.Database.DB.Query(q)
  if nil != err {
    return err
  }
  defer rows.Close()

  // Marshal rows
  for rows.Next() {
    if err = rows.Scan(&head.ID, &head.Title, &head.Subtitle, &head.Tag, &head.Created,
      &head.Updated); nil != err {
        break
      } else {
        list = append(list, head)
      }
  }

  // Check error
  if nil != err {
    return err
  }

  // Write to buffer and return any encoding error
  return re.Marshal(route.ContentTypeJSON, &list)
}

func (c *ListController) Post (x context.Context, rq *http.Request, re *route.Result) error {
  return re.Unimplemented()
}


func (c *ListController) Put (x context.Context, rq *http.Request, re *route.Result) error {
  return re.Unimplemented()
}

func (c *ListController) Delete (x context.Context, rq *http.Request, re *route.Result) error {
  return re.Unimplemented()
}
