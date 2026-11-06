package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kron-smart-chain/webwallet/internal/models"
)

type addressBookRequest struct {
	Label   string `json:"label"`
	Address string `json:"address"`
}

func (s *Server) listAddressBook(c *gin.Context) {
	user := currentUser(c)
	var entries []models.AddressBookEntry
	if err := s.db.WithContext(c.Request.Context()).Where("user_id = ?", user.ID).Order("label ASC").Find(&entries).Error; err != nil {
		internal(c, err)
		return
	}
	respondOK(c, gin.H{"entries": entries})
}

func (s *Server) addAddressBook(c *gin.Context) {
	user := currentUser(c)
	var req addressBookRequest
	if !bindJSON(c, &req) {
		return
	}
	label := strings.TrimSpace(req.Label)
	if label == "" || len(label) > 120 {
		fail(c, http.StatusBadRequest, "invalid_label", "Label is required and must be at most 120 characters.")
		return
	}
	if !s.validateAddress(c, req.Address) {
		return
	}
	var count int64
	_ = s.db.WithContext(c.Request.Context()).Model(&models.AddressBookEntry{}).Where("user_id = ?", user.ID).Count(&count).Error
	if count >= 200 {
		fail(c, http.StatusBadRequest, "address_book_full", "Address book limit reached.")
		return
	}
	entry := models.AddressBookEntry{UserID: user.ID, Label: label, Address: strings.TrimSpace(req.Address)}
	if err := s.db.WithContext(c.Request.Context()).Create(&entry).Error; err != nil {
		internal(c, err)
		return
	}
	audit(c, &user.ID, "address_book.added", "address="+entry.Address)
	created(c, entry)
}

func (s *Server) deleteAddressBook(c *gin.Context) {
	user := currentUser(c)
	id, valid := idParam(c, "id")
	if !valid {
		return
	}
	res := s.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", id, user.ID).Delete(&models.AddressBookEntry{})
	if res.Error != nil {
		internal(c, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		fail(c, http.StatusNotFound, "not_found", "Entry not found.")
		return
	}
	audit(c, &user.ID, "address_book.removed", "id="+c.Param("id"))
	respondOK(c, gin.H{"removed": true})
}
