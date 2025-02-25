package models

import (
	"encoding/json"
	"io"
	"time"
)

type (
	UserRegistration struct {
		Name     string `json:"login"`
		Password string `json:"password"`
	}

	Order struct {
		OrderID string    `json:"number"`
		Status  string    `json:"status"`
		Accrual float64   `json:"accrual,omitempty"`
		Time    time.Time `json:"uploaded_at"`
	}

	OrderAccrual struct {
		OrderID string  `json:"order"`
		Status  string  `json:"status"`
		Accrual float64 `json:"accrual,omitempty"`
	}

	Balance struct {
		Accrual   float64 `json:"current"`
		Withdrawn float64 `json:"withdrawn"`
	}

	Withdraw struct {
		OrderID string    `json:"order"`
		Sum     float64   `json:"sum"`
		TimeC   time.Time `json:"processed_at,omitempty"`
	}
)

func (val *Withdraw) FromJSON(body io.ReadCloser) error {
	err := json.NewDecoder(body).Decode(val)
	return err
}

func (val *UserRegistration) FromJSON(body io.ReadCloser) error {
	err := json.NewDecoder(body).Decode(val)
	return err
}
func (val *UserRegistration) ToJSON(w io.Writer) error {
	err := json.NewEncoder(w).Encode(val)
	return err
}

func (val *OrderAccrual) FromJSON(body io.ReadCloser) error {
	err := json.NewDecoder(body).Decode(val)
	return err
}

func JSONSEncodeBytes(w io.Writer, val any /*val []Order*/) error {
	enc := json.NewEncoder(w)
	err := enc.Encode(val)
	return err
}
