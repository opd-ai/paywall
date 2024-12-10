package main

import (
	"net/http"

	"github.com/opd-ai/paywall"
)

func main() {
	pw, err := paywall.NewPaywall(0.001, true)
	if err != nil {
		panic(err)
	}

	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Protected content"))
	})

	http.Handle("/protected", pw.Middleware(protected))
	http.ListenAndServe(":8000", nil)
}
