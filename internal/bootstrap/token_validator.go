package bootstrap

import (
	"errors"
	"log/slog"

	"github.com/augustdev/autoclip/internal/authz"
)

func NewTokenValidator(cfg GraphQLAPIConfig, logger *slog.Logger) (authz.TokenValidator, error) {
	switch cfg.ValidatorType {
	case "jwk":
		tokenValidator, err := authz.NewTokenValidator(cfg.JWTJWKSURL)
		if err != nil {
			logger.Error("Failed to create JWT token validator", slog.Any("error", err))
			return nil, err
		}
		return tokenValidator, nil

	default:
		logger.Error("Invalid validator type", "validator_type", cfg.ValidatorType)
		return nil, errors.New("validator type must be 'firebase', 'jwk', or 'test'")
	}
}
