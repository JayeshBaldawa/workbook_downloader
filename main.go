package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"net/http"
	"sync"

	"github.com/jung-kurt/gofpdf"
)

// Configurable constants
const (
	ImageURLTemplate = "XX/%d.XX" // Replace XX with actual URL
	FirstImageIndex  = 1
	LastImageIndex   = 56
	WorkerCount      = 10 // Number of concurrent goroutines
)

// ImageData holds image and index info for proper ordering
type ImageData struct {
	Index int
	Image image.Image
	Error error
}

// DownloadImage downloads an image and converts it to 8-bit depth if needed.
func DownloadImage(url string) (image.Image, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %v", err)
	}
	defer resp.Body.Close()

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}

	return convertTo8Bit(img), nil
}

// convertTo8Bit ensures the image is 8-bit (no 16-bit depth issues)
func convertTo8Bit(img image.Image) image.Image {
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)
	return rgba
}

// Worker function to download images concurrently
func worker(id int, jobs <-chan int, results chan<- ImageData, wg *sync.WaitGroup) {
	defer wg.Done()

	for index := range jobs {
		url := fmt.Sprintf(ImageURLTemplate, index)
		fmt.Printf("Worker %d: Downloading image %d...\n", id, index)

		img, err := DownloadImage(url)
		results <- ImageData{Index: index, Image: img, Error: err}
	}
}

// AddImageToPDF adds an image to the PDF with a unique identifier.
func AddImageToPDF(pdf *gofpdf.Fpdf, img image.Image, index int) error {
	imgBuffer := new(bytes.Buffer)
	err := png.Encode(imgBuffer, img)
	if err != nil {
		return fmt.Errorf("failed to encode image: %v", err)
	}

	pdf.AddPage()

	width := float64(img.Bounds().Dx()) / 4
	height := float64(img.Bounds().Dy()) / 4

	imageName := fmt.Sprintf("image_%d.png", index) // Unique image name
	pdf.RegisterImageReader(imageName, "PNG", bytes.NewReader(imgBuffer.Bytes()))
	pdf.Image(imageName, 10, 10, width, height, false, "", 0, "")

	return nil
}

func main() {
	pdf := gofpdf.New("P", "mm", "A4", "")

	jobs := make(chan int, LastImageIndex-FirstImageIndex+1)
	results := make(chan ImageData, LastImageIndex-FirstImageIndex+1)

	var wg sync.WaitGroup

	// Start worker pool
	for i := 1; i <= WorkerCount; i++ {
		wg.Add(1)
		go worker(i, jobs, results, &wg)
	}

	// Add jobs to the channel
	for i := FirstImageIndex; i <= LastImageIndex; i++ {
		jobs <- i
	}
	close(jobs)

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results and maintain order
	imageMap := make(map[int]image.Image)
	for data := range results {
		if data.Error != nil {
			fmt.Printf("Error downloading image %d: %v\n", data.Index, data.Error)
			continue
		}
		imageMap[data.Index] = data.Image
	}

	// Add images to PDF in the correct order
	for i := FirstImageIndex; i <= LastImageIndex; i++ {
		if img, exists := imageMap[i]; exists {
			err := AddImageToPDF(pdf, img, i)
			if err != nil {
				fmt.Printf("Error adding image %d to PDF: %v\n", i, err)
			}
		}
	}

	// Save the PDF
	err := pdf.OutputFileAndClose("final_output.pdf")
	if err != nil {
		fmt.Printf("Error saving PDF: %v\n", err)
	}

	fmt.Println("PDF created successfully as final_output.pdf")
}
