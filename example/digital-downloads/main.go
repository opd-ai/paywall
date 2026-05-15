package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/opd-ai/paywall"
)

func main() {
	store := paywall.NewMemoryStore()

	pw, err := paywall.NewPaywall(paywall.Config{
		PriceInBTC:       0.0001,
		TestNet:          true,
		Store:            store,
		PaymentTimeout:   time.Hour * 24,
		MinConfirmations: 1,
	})
	if err != nil {
		log.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	downloadsDir := "./downloads"
	if err := os.MkdirAll(downloadsDir, 0o755); err != nil {
		log.Fatalf("Failed to create downloads directory: %v", err)
	}

	if err := createSampleFiles(downloadsDir); err != nil {
		log.Fatalf("Failed to create sample files: %v", err)
	}

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/catalog", catalogHandler(downloadsDir))
	http.Handle("/download/", pw.Middleware(http.HandlerFunc(downloadHandler(downloadsDir))))

	log.Println("Digital downloads server running on :8080")
	log.Println("Visit http://localhost:8080 to browse files")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head><title>Digital Downloads</title></head>
<body>
	<h1>Digital Downloads Marketplace</h1>
	<p>Browse and purchase digital files with Bitcoin.</p>
	<ul>
		<li><a href="/catalog">View Catalog</a></li>
	</ul>
</body>
</html>
`)
}

func catalogHandler(downloadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		files, err := os.ReadDir(downloadsDir)
		if err != nil {
			http.Error(w, "Failed to read catalog", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head><title>Download Catalog</title></head>
<body>
	<h1>Available Downloads</h1>
	<ul>
`)
		for _, file := range files {
			if !file.IsDir() {
				fmt.Fprintf(w, `<li><a href="/download/%s">%s</a> (0.0001 BTC)</li>`, file.Name(), file.Name())
			}
		}
		fmt.Fprintf(w, `
	</ul>
	<p><a href="/">Back to Home</a></p>
</body>
</html>
`)
	}
}

func downloadHandler(downloadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filename := filepath.Base(r.URL.Path)
		filePath := filepath.Join(downloadsDir, filename)

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}

		file, err := os.Open(filePath)
		if err != nil {
			http.Error(w, "Failed to open file", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
		w.Header().Set("Content-Type", "application/octet-stream")

		if _, err := io.Copy(w, file); err != nil {
			log.Printf("Failed to send file: %v", err)
		}
	}
}

func createSampleFiles(dir string) error {
	sampleFiles := map[string]string{
		"ebook.pdf":    "Sample PDF eBook content - Lorem ipsum dolor sit amet...",
		"music.mp3":    "Sample MP3 audio file - Binary data would go here...",
		"software.zip": "Sample software package - Compressed files would go here...",
		"template.psd": "Sample Photoshop template - PSD data would go here...",
	}

	for filename, content := range sampleFiles {
		filePath := filepath.Join(dir, filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
				return fmt.Errorf("failed to create %s: %w", filename, err)
			}
		}
	}
	return nil
}
