package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"golang.org/x/net/proxy"
)

// Ayarlar
const (
	torProxyAddr = "127.0.0.1:9050" // Tor SOCKS5 Portu
	outputBase   = "scans"          // Ana çıktı klasörü
)

func main() {
	fmt.Println(`
███████████ █████   █████    ███████    ███████████  
░█░░░███░░░█░░███   ░░███   ███░░░░░███ ░░███░░░░░███ 
░   ░███  ░  ░███    ░███  ███     ░░███ ░███    ░███ 
    ░███     ░███████████ ░███      ░███ ░██████████  
    ░███     ░███░░░░░███ ░███      ░███ ░███░░░░░███ 
    ░███     ░███    ░███ ░░███     ███  ░███    ░███ 
    █████    █████   █████ ░░░███████░   █████   █████
   ░░░░░    ░░░░░   ░░░░░    ░░░░░░░    ░░░░░   ░░░░░
`)

	// 1. Konsoldan veri okumak için okuyucuyu hazırla
	reader := bufio.NewReader(os.Stdin)

	// 2. Ekrana tam istediğin mesajı yazdır
	// (Println yerine Print kullanıyoruz ki imleç yanıp sönsün)
	fmt.Print("hedef bilgisi girin: ")

	// 3. Kullanıcının bir şey yazıp Enter'a basmasını bekle
	input, _ := reader.ReadString('\n')

	// 4. Girilen verinin başındaki/sonundaki gereksiz boşlukları temizle
	target := strings.TrimSpace(input)

	// 5. Hedef girilmiş mi kontrol et
	if target != "" {
		fmt.Printf("\n[%s] hedefi algılandı, işlemler başlatılıyor...\n", target)

		// ==========================================
		// PDF'TEKİ ASIL TARAMA KODLARIN BURAYA GELECEK
		// Örneğin fonksiyonun adı 'baslat' ise:
		// Baslat(target)
		runScraper(target)
		// ==========================================

	} else {
		fmt.Println("Hata: Hedef bilgisi girmediniz.")
	}

	// 6. İşlem bitince konsol hemen kapanmasın, sonucu gör diye bekletme
	fmt.Println("\nProgramı kapatmak için Enter'a basın.")
	reader.ReadString('\n')
}

func runScraper(targetPath string) {
	fmt.Println("\n[*] Tarama Başlatılıyor...")
	fmt.Printf("[INIT] Hedef Dosyası: %s\n", targetPath)

	// 2. Tor Proxy Ayarları
	dialer, err := proxy.SOCKS5("tcp", torProxyAddr, nil, proxy.Direct)
	if err != nil {
		log.Printf("[FATAL] Tor proxy hatası: %v", err)
		return
	}
	httpClient := &http.Client{
		Transport: &http.Transport{Dial: dialer.Dial},
		Timeout:   time.Second * 45, // Onion siteleri yavaştır, süreyi uzattık
	}

	// Ana klasörü oluştur
	if _, err := os.Stat(outputBase); os.IsNotExist(err) {
		os.Mkdir(outputBase, 0755)
	}

	// 3. Dosya Okuma (Artık parametreden gelen dosyayı okuyoruz)
	file, err := os.Open(targetPath)
	if err != nil {
		log.Printf("[FATAL] Dosya açılamadı: %v", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		rawUrl := strings.TrimSpace(scanner.Text())
		if rawUrl == "" {
			continue
		}

		fmt.Printf("\n[INFO] Hedef Taranıyor: %s\n", rawUrl)

		// Klasör ismini URL'den türet (http ve .onion kısımlarını temizleyerek)
		folderName := sanitizeFolderName(rawUrl)
		targetDir := fmt.Sprintf("%s/%s", outputBase, folderName)

		// Hedefe özel klasör oluştur
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			fmt.Printf("[ERR] Klasör oluşturulamadı: %v\n", err)
			continue
		}

		// A) HTML İndir ve .txt olarak kaydet
		fmt.Print("   |-- HTML Çekiliyor... ")
		err = downloadHTML(httpClient, rawUrl, targetDir, folderName)
		if err != nil {
			fmt.Printf("BAŞARISIZ (%v)\n", err)
		} else {
			fmt.Println("BAŞARILI")
		}

		// B) Ekran Görüntüsü Al (Chromedp ile)
		fmt.Print("   |-- Screenshot Alınıyor... ")
		err = takeScreenshot(rawUrl, targetDir, folderName)
		if err != nil {
			fmt.Printf("BAŞARISIZ (%v)\n", err)
		} else {
			fmt.Println("BAŞARILI")
		}
	}
	fmt.Println("\n[*] Tarama Tamamlandı.")
}

// downloadHTML: Site kaynağını indirip .txt olarak kaydeder
func downloadHTML(client *http.Client, url, dir, name string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Dosya yolu: scans/hedef_adı/hedef_adı.txt
	filename := fmt.Sprintf("%s/%s.txt", dir, name)
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

// takeScreenshot: Chromedp kullanarak Tor üzerinden ekran görüntüsü alır
func takeScreenshot(urlStr, dir, name string) error {
	// Chromedp ayarları: Tor Proxy kullanımı
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ProxyServer("socks5://"+torProxyAddr), // Tor üzerinden git
		chromedp.WindowSize(1280, 720),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// Timeoutlu bir context oluştur (30 saniye bekle, açılmazsa geç)
	ctx, cancel := context.WithTimeout(allocCtx, 45*time.Second)
	defer cancel()

	ctx, cancel = chromedp.NewContext(ctx)
	defer cancel()

	var buf []byte

	// Tarayıcı işlemleri
	err := chromedp.Run(ctx,
		chromedp.Navigate(urlStr),
		chromedp.Sleep(2*time.Second), // Sayfanın tam yüklenmesi için bekle
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		return err
	}

	// Resmi kaydet: scans/hedef_adı/hedef_adı.png
	filename := fmt.Sprintf("%s/%s.png", dir, name)
	if err := os.WriteFile(filename, buf, 0644); err != nil {
		return err
	}
	return nil
}

// sanitizeFolderName: URL'den temiz bir klasör ismi çıkarır
func sanitizeFolderName(url string) string {
	// http://, https:// ve / işaretlerini temizle
	name := strings.ReplaceAll(url, "http://", "")
	name = strings.ReplaceAll(name, "https://", "")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, ":", "_")
	// Uzunsa kısalt
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}
