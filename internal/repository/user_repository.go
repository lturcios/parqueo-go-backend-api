package repository

import (
	"github.com/parqueo/api/internal/database"
	"github.com/parqueo/api/internal/domain/models"
)

func GetUserByEmail(email string) (*models.User, error) {
	var user models.User
	err := database.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
