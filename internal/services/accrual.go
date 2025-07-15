package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gophermart/internal/models"
)

type AccrualService struct {
	client *http.Client
	address string
}

func NewAccrualService(client *http.Client, address string) *AccrualService {
	return &AccrualService{
		client:  client,
		address: address,
	}
}

func (s *AccrualService) GetAccrual(ctx context.Context, orderNumber string) (*models.AccrualResponse, error) {
	url := fmt.Sprintf("%s/api/orders/%s", s.address, orderNumber)
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var accrual models.AccrualResponse
		if err := json.NewDecoder(resp.Body).Decode(&accrual); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return &accrual, nil
	case http.StatusNoContent:
		return nil, fmt.Errorf("order not found in accrual system")
	case http.StatusTooManyRequests:
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter == "" {
			retryAfter = "60"
		}
		delay, err := strconv.Atoi(retryAfter)
		if err != nil {
			delay = 60
		}
		time.Sleep(time.Duration(delay) * time.Second)
		return s.GetAccrual(ctx, orderNumber)
	default:
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}