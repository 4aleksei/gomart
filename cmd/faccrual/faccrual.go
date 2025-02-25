package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type OrderAccrual struct {
	OrderID string  `json:"order"`
	Status  string  `json:"status"`
	Accrual float64 `json:"accrual,omitempty"`
}

func JSONSEncodeBytes(w io.Writer, val any) error {
	enc := json.NewEncoder(w)
	err := enc.Encode(val)
	return err
}

func handle_http(res http.ResponseWriter, req *http.Request) {

	id := req.PathValue("number")
	log.Print("read order ", id)

	var order OrderAccrual
	order.OrderID = id

	switch id[:1] {

	case "1":
		order.Status = "INVALID"

	default:
		order.Status = "PROCESSED"
		order.Accrual = 700

	}

	res.Header().Add("Content-Type", "application/json")

	var buf bytes.Buffer
	if errson := JSONSEncodeBytes(io.Writer(&buf), order); errson != nil {
		log.Print("error encoding response ", errson)
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	res.WriteHeader(http.StatusOK)

	if _, err := io.WriteString(res, buf.String()); err != nil {
		log.Print("error writing response", err)
		res.WriteHeader(http.StatusInternalServerError)
		return
	} else {
		fmt.Println(buf.String())
	}
}

func main() {
	http.HandleFunc("/api/orders/{number}", handle_http)
	http.ListenAndServe(":8100", nil)
}
