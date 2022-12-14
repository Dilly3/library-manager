package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "os"
	"strings"
	"time"

	jwtauth "github.com/dilly3/library-manager/auth"
	"github.com/dilly3/library-manager/database"
	"github.com/dilly3/library-manager/models"
	utils "github.com/dilly3/library-manager/utils"
	"github.com/gin-gonic/gin"
	_ "github.com/golang-jwt/jwt"
	"go.uber.org/zap"
)

type Handle struct {
	storeBK  database.DataStore
	storeUSR database.UserStore
	Logger   *zap.Logger
}

func NewHandle(databaseFactory func() database.UserStore, databaseFactory2 func() database.DataStore) *Handle {
	db := databaseFactory2()
	db2 := databaseFactory()

	return &Handle{
		storeBK:  db,
		storeUSR: db2,
		Logger:   zap.NewExample(),
	}
}

func (h *Handle) Home() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.File("templates/index2.htm")
	}
}

func (h *Handle) UserSignUp() gin.HandlerFunc {
	return func(c *gin.Context) {
		//get request body
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			errorMessage := "error reading request body"
			h.Logger.Fatal(errorMessage, zap.Error(err))
			c.JSON(http.StatusInternalServerError, utils.ErrorResponse{
				Code:    http.StatusBadRequest,
				Error:   fmt.Sprintf("%s", errors.New(errorMessage)),
				Message: errorMessage,
			})
			return
		}
		defer c.Request.Body.Close()

		var user *models.User
		err = json.Unmarshal(body, &user)
		if err != nil {
			errorMessage := "failed to unmarshall user registration object"
			h.Logger.Error(errorMessage, zap.Error(err))
			c.JSON(http.StatusOK, utils.ErrorResponse{
				Code:    http.StatusBadRequest,
				Error:   fmt.Sprintf("%s", errors.New(errorMessage)),
				Message: errorMessage,
			})
			return
		}

		if user.Email == "" || user.UserName == "" || user.Password == nil {
			errorMessage := "empty request object"
			c.JSON(http.StatusOK, utils.ErrorResponse{
				Code:    http.StatusBadRequest,
				Error:   fmt.Sprintf("%s\n", errors.New(errorMessage)),
				Message: errorMessage,
			})
			return

		}
		user.CreatedAt = utils.GetPresentTime()
		user.UpdatedAt = utils.GetPresentTime()
		user.Role = models.LIBRARY_USER
		user.Password = utils.EncryptPassword(user.Password)

		//check if valid new user
		ok := h.storeUSR.CheckUserByEmail(user.Email)
		if ok {
			errorMessage := "user already exists"
			c.JSON(http.StatusOK, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   errorMessage,
				Message: errorMessage,
			})
			return
		}
		user, err = h.storeUSR.CreateUser(user)
		if err != nil {
			errorMessage := "error creating user"
			h.Logger.Error(errorMessage, zap.Error(err))
			c.JSON(http.StatusOK, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   errorMessage,
				Message: errorMessage,
			})
			return
		}
		c.JSON(http.StatusOK, utils.SuccessResponse{
			Code:    http.StatusOK,
			Object:  user,
			Message: "User created successfully",
		})

	}
}

func (h *Handle) UserLogin() gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			errorMessage := "error reading request body"
			h.Logger.Fatal(errorMessage, zap.Error(err))
			c.JSON(http.StatusBadRequest, utils.ErrorResponse{
				Code:    http.StatusBadRequest,
				Error:   fmt.Sprintf("%s", errors.New(errorMessage)),
				Message: errorMessage,
			})
			return
		}
		defer c.Request.Body.Close()

		var user models.User
		err = json.Unmarshal(body, &user)
		if err != nil {
			errorMessage := "failed to unmarshall user login object"
			h.Logger.Error(errorMessage, zap.Error(err))
			c.JSON(http.StatusBadRequest, utils.ErrorResponse{
				Code:    http.StatusBadRequest,
				Error:   fmt.Sprintf("%s", errors.New(errorMessage)),
				Message: errorMessage,
			})
			return
		}

		if user.Email == "" || user.Password == nil {
			errorMessage := "empty request object"
			c.JSON(http.StatusOK, utils.ErrorResponse{
				Code:    http.StatusBadRequest,
				Error:   fmt.Sprintf("%s\n", errors.New(errorMessage)),
				Message: errorMessage,
			})
			return

		}
		userData, err := h.storeUSR.GetUserByEmail(user.Email)
		if err != nil {
			errorMessage := "error getting user by email"
			h.Logger.Error(errorMessage, zap.Error(err))
			c.JSON(http.StatusOK, utils.ErrorResponse{
				Code:    http.StatusForbidden,
				Error:   fmt.Sprintf("%s\n", errors.New(errorMessage)),
				Message: "user does not exist",
			})
			return
		}

		passwordMatch := utils.ComparePasscode(*user.Password, *userData.Password)
		if !passwordMatch {
			errorMessage := "invalid login credentials"
			h.Logger.Error(errorMessage, zap.Error(err))
			c.JSON(http.StatusOK, utils.ErrorResponse{
				Code:    http.StatusForbidden,
				Error:   fmt.Sprintf("%s\n", errors.New(errorMessage)),
				Message: "invalid login credentials",
			})
			return
		}

		claims := &jwtauth.UserClaims{
			Name:      userData.ID,
			Email:     userData.Email,
			Role:      userData.Role,
			SessionID: 1,
			ExpireAt:  time.Now().Add(time.Hour * 24).Unix(),
		}

		tokenString, err := jwtauth.GenToken(claims)
		if err != nil {
			errorMessage := "error creating token"
			h.Logger.Error(errorMessage, zap.Error(err))
			c.JSON(http.StatusOK, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   errorMessage,
				Message: fmt.Sprintf("%s\n", errors.New(errorMessage)),
			})
			return

		}

		time.Sleep(time.Millisecond * 500)

		c.SetCookie("jwt", *tokenString, 3600, "/", "", false, true)

		c.JSON(http.StatusOK, utils.SuccessResponse{
			Code:    http.StatusOK,
			Object:  tokenString,
			Message: "User logged in successfully",
		})
	}
}

func (h *Handle) CreateBook() gin.HandlerFunc {
	return func(c *gin.Context) {

		reqToken := c.GetHeader("Authorization")
		splitToken := strings.Split(reqToken, "Bearer")

		if len(splitToken) != 2 || len(splitToken[1]) < 1 {
			errorMessage := "error getting authorization token"
			c.JSON(http.StatusUnauthorized, utils.ErrorResponse{
				Code:    http.StatusUnauthorized,
				Error:   errorMessage,
				Message: errorMessage,
			})
			return
		}

		reqToken = strings.TrimSpace(splitToken[1])

		//token clamins contains user email
		claims, err := jwtauth.ParseToken(reqToken)

		if err != nil {
			errorMessage := "error validating token string"
			h.Logger.Error(errorMessage, zap.Error(err))
			c.JSON(http.StatusUnauthorized, utils.ErrorResponse{
				Code:    http.StatusUnauthorized,
				Error:   errorMessage,
				Message: errorMessage,
			})
			return
		}

		if claims.Role != models.ADMIN_USER {
			c.JSON(http.StatusUnauthorized, utils.ErrorResponse{
				Code:    http.StatusUnauthorized,
				Error:   "unauthorized",
				Message: "unauthorized",
			})
			return
		}

		var book = new(models.Book)
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, int64(models.MAX_SIZE)))
		if err != nil {
			log.Println(err)
			c.JSON(500, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   "Error reading request body",
				Message: fmt.Sprint(err),
			})
			return
		}

		err = json.Unmarshal(body, &book)
		if err != nil {
			log.Println(err)
			c.JSON(500, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   "Error unmarshalling request body",
				Message: fmt.Sprint(err),
			})
			return
		}
		ok, err := h.storeBK.IsBookInStore(*book.Title, *book.Author)
		if err != nil {
			h.Logger.Info("Error validating book name", zap.Error(err))
			c.JSON(500, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   "Error validating book name",
				Message: fmt.Sprint(err),
			})
			return
		}

		if ok {
			c.JSON(500, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   "Book name already exists",
				Message: fmt.Sprint(err),
			})
			return
		}
		book.Status = models.AVAILABLE
		book.RentedBy = &models.UserInfo{}
		newBook, err := h.storeBK.AddBook(book)
		if err != nil {
			log.Println(err)
			c.JSON(500, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   "Error creating book",
				Message: fmt.Sprint(err),
			})
			return
		}
		c.JSON(200, utils.SuccessResponse{
			Code:    http.StatusCreated,
			Object:  newBook,
			Message: "Book created successfully",
		})

	}
}

func (h *Handle) GetBook() gin.HandlerFunc {
	return func(c *gin.Context) {

		bookId := c.Param("book_id")

		var book *models.Book

		//objectId, _ := utils.GetPrimitiveObjectId(bookId)

		book, err := h.storeBK.GetBook(bookId)
		if err != nil {
			h.Logger.Info("Error getting book", zap.Error(err))
			c.JSON(500, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   "Error getting book",
				Message: fmt.Sprint(err),
			})
			return
		}
		c.JSON(200, utils.SuccessResponse{
			Code:    http.StatusOK,
			Object:  book,
			Message: "Book retrieved successfully",
		})
	}

}

func (h *Handle) UpdateBook() gin.HandlerFunc {
	return func(c *gin.Context) {

		reqToken := c.GetHeader("Authorization")
		splitToken := strings.Split(reqToken, "Bearer")

		if len(splitToken) != 2 || len(splitToken[1]) < 1 {
			errorMessage := "error getting authorization token"
			c.JSON(http.StatusUnauthorized, utils.ErrorResponse{
				Code:    http.StatusUnauthorized,
				Error:   errorMessage,
				Message: errorMessage,
			})
			return
		}

		reqToken = strings.TrimSpace(splitToken[1])

		claims, err := jwtauth.ParseToken(reqToken)

		if err != nil {
			errorMessage := "error validating token string"
			h.Logger.Error(errorMessage, zap.Error(err))
			c.JSON(http.StatusUnauthorized, utils.ErrorResponse{
				Code:    http.StatusUnauthorized,
				Error:   errorMessage,
				Message: errorMessage,
			})
			return
		}

		if claims.Role != models.ADMIN_USER {
			c.JSON(http.StatusUnauthorized, utils.ErrorResponse{
				Code:    http.StatusUnauthorized,
				Error:   "unauthorized",
				Message: "unauthorized",
			})
			return
		}

		bookId := c.Param("book_id")
		var book models.Book
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, int64(models.MAX_SIZE)))
		if err != nil {
			log.Println(err)
			c.JSON(500, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   "Error reading request body",
				Message: fmt.Sprint(err),
			})
			return
		}
		err = json.Unmarshal(body, &book)
		if err != nil {
			h.Logger.Info("Error unmarshalling request body", zap.Error(err))
			c.JSON(500, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   "Error unmarshalling request body",
				Message: fmt.Sprint(err),
			})
			return
		}

		//objectId, _ := utils.GetPrimitiveObjectId(bookId)
		_, err = h.storeBK.UpdateBook(bookId, &book)
		if err != nil {
			h.Logger.Info("Error updating book", zap.Error(err))
			c.JSON(500, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   "Error updating book",
				Message: fmt.Sprint(err),
			})
			return
		}
		updatedBook, errStr := h.storeBK.GetBook(bookId)
		if errStr != nil {
			h.Logger.Info("Error getting book", zap.Error(errStr))
			c.JSON(500, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   "Error getting book",
				Message: fmt.Sprint(err),
			})
			return
		}

		c.JSON(200, utils.SuccessResponse{
			Code:    http.StatusOK,
			Object:  updatedBook,
			Message: "Book updated successfully",
		})
	}
}

func (h *Handle) GetAllBooks() gin.HandlerFunc {
	return func(c *gin.Context) {
		books, err := h.storeBK.GetAllBooks()
		if err != nil {
			h.Logger.Info("Error getting all books", zap.Error(err))
			c.JSON(500, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   "Error getting all books",
				Message: fmt.Sprint(err),
			})
			return
		}
		c.JSON(200, utils.SuccessResponse{
			Code:    http.StatusOK,
			Object:  books,
			Message: "Books retrieved successfully",
		})

	}
}

func (h *Handle) DeleteBook() gin.HandlerFunc {
	return func(c *gin.Context) {

		reqToken := c.GetHeader("Authorization")
		splitToken := strings.Split(reqToken, "Bearer")

		if len(splitToken) != 2 || len(splitToken[1]) < 1 {
			errorMessage := "error getting authorization token"
			c.JSON(http.StatusUnauthorized, utils.ErrorResponse{
				Code:    http.StatusUnauthorized,
				Error:   errorMessage,
				Message: errorMessage,
			})
			return
		}

		reqToken = strings.TrimSpace(splitToken[1])

		//token clamins contains user email
		claims, err := jwtauth.ParseToken(reqToken)

		if err != nil {
			errorMessage := "error validating token string"
			h.Logger.Error(errorMessage, zap.Error(err))
			c.JSON(http.StatusUnauthorized, utils.ErrorResponse{
				Code:    http.StatusUnauthorized,
				Error:   errorMessage,
				Message: errorMessage,
			})
			return
		}

		if claims.Role != models.ADMIN_USER {
			c.JSON(http.StatusUnauthorized, utils.ErrorResponse{
				Code:    http.StatusUnauthorized,
				Error:   "unauthorized",
				Message: "unauthorized",
			})
			return
		}

		bookId := c.Param("_id")

		//objectId, _ := utils.GetPrimitiveObjectId(bookId)
		book, err := h.storeBK.GetBook(bookId)
		if err != nil {
			h.Logger.Info("Error getting book", zap.Error(err))
			c.JSON(500, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   "Error getting book",
				Message: fmt.Sprint(err),
			})
			return
		}
		err2 := h.storeBK.DeleteBook(bookId)
		if err2 != nil {
			h.Logger.Info("Error deleting book", zap.Error(err))
			c.JSON(500, utils.ErrorResponse{
				Code:    http.StatusInternalServerError,
				Error:   "Error deleting book",
				Message: fmt.Sprint(err),
			})
			return
		}
		c.JSON(200, utils.SuccessResponse{
			Code:    http.StatusOK,
			Message: fmt.Sprintf("%s deleted successfully", *book.Title),
		})

	}
}
