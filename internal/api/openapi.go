package api

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed openapi.json
var openapiSpec []byte

//go:embed llms.txt
var llmsGuide []byte

func (s *Server) openapiJSON(c *gin.Context) {
	c.Data(http.StatusOK, "application/json; charset=utf-8", openapiSpec)
}

func (s *Server) llmsTXT(c *gin.Context) {
	c.Data(http.StatusOK, "text/plain; charset=utf-8", llmsGuide)
}
