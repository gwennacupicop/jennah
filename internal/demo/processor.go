package demo

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProcessResult holds the outcome of processing
type ProcessResult struct {
	Metrics *ProcessMetrics
	Error   error
}

// Processor handles file processing with error recovery
type Processor struct {
	config       *DistributedConfig
	errorHandler *ErrorHandler
}

// NewProcessor creates processor instance
func NewProcessor(cfg *DistributedConfig) *Processor {
	return &Processor{
		config:       cfg,
		errorHandler: NewErrorHandler(),
	}
}

// Process reads byte range from file and counts lines/words
func (p *Processor) Process(ctx context.Context) (*ProcessMetrics, error) {
	metrics := &ProcessMetrics{
		InstanceID:   p.config.InstanceID,
		TaskIndex:    p.config.InstanceID,
		TaskIndexMax: p.config.TotalInstances - 1,
		JobID:        p.config.JobID,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}

	// Calculate byte range
	calc := NewChunkCalculator(p.config.InputDataSize, p.config.TotalInstances)
	byteRange, err := calc.Calculate(p.config.InstanceID)
	if err != nil {
		return metrics, fmt.Errorf("chunk calculation failed: %w", err)
	}

	metrics.StartByte = byteRange.StartByte
	metrics.EndByte = byteRange.EndByte
	metrics.BytesProcessed = byteRange.Size()

	// Skip processing if range is empty
	if byteRange.IsEmpty() {
		log.Printf("Instance %d: No work (file too small)", p.config.InstanceID)
		return metrics, nil
	}

	// Open file with retry
	var reader io.Reader
	err = p.errorHandler.HandleWithRetry(func() error {
		r, err := p.openFile(ctx)
		if err != nil {
			return err
		}
		reader = r
		return nil
	}, "File open")

	if err != nil {
		return metrics, err
	}

	// Process with error handling
	err = p.processReader(ctx, reader, byteRange, metrics)
	if err != nil {
		return metrics, err
	}

	return metrics, nil
}

// processReader reads from the given range and counts lines/words
func (p *Processor) processReader(ctx context.Context, reader io.Reader, br *ByteRange, m *ProcessMetrics) error {
	var limitedReader io.Reader

	// For local files, seek to start byte and create limited reader.
	// For GCS/other readers, the byte range is already handled by the range reader.
	if file, ok := reader.(*os.File); ok {
		_, err := file.Seek(br.StartByte, io.SeekStart)
		if err != nil {
			p.errorHandler.HandleError(ErrProcessing, err)
			return fmt.Errorf("seek to byte %d failed: %w", br.StartByte, err)
		}
		limitedReader = io.LimitReader(file, br.Size())
	} else {
		// GCS range reader already returns only the requested byte range
		limitedReader = reader
	}

	// Count lines and words
	scanner := bufio.NewScanner(limitedReader)
	// Set buffer size to handle long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineCount := int64(0)
	wordCount := int64(0)
	charCount := int64(0)

	for scanner.Scan() {
		line := scanner.Text()
		lineCount++

		// Count characters (UTF-8 encoded)
		charCount += int64(len([]rune(line)))

		// Count words (split by whitespace)
		words := strings.Fields(line)
		wordCount += int64(len(words))
	}

	if err := scanner.Err(); err != nil {
		p.errorHandler.HandleError(ErrProcessing, err)
		return fmt.Errorf("scan failed: %w", err)
	}

	// Update metrics
	m.LinesCount = lineCount
	m.WordsCount = wordCount
	m.CharactersCount = charCount

	log.Printf("Instance %d processed: %d lines, %d words, %d chars in %d bytes",
		p.config.InstanceID, lineCount, wordCount, charCount, br.Size())

	return nil
}

// openFile opens the input file (local or GCS)
func (p *Processor) openFile(ctx context.Context) (io.Reader, error) {
	// Check if GCS path
	if IsGCSPath(p.config.InputDataPath) {
		return p.openGCSFile(ctx)
	}

	// Local file path
	fileInfo, err := os.Stat(p.config.InputDataPath)
	if err != nil {
		if os.IsNotExist(err) {
			p.errorHandler.HandleError(ErrFileNotFound, err)
			return nil, fmt.Errorf("input file not found: %s", p.config.InputDataPath)
		}
		p.errorHandler.HandleError(ErrPermissionDenied, err)
		return nil, fmt.Errorf("cannot access input file: %w", err)
	}

	// If InputDataSize not set, use actual file size
	if p.config.InputDataSize == 0 {
		p.config.InputDataSize = fileInfo.Size()
		log.Printf("Using actual file size: %d bytes", p.config.InputDataSize)
	}

	file, err := os.Open(p.config.InputDataPath)
	if err != nil {
		p.errorHandler.HandleError(ErrFileNotFound, err)
		return nil, fmt.Errorf("cannot open input file: %w", err)
	}

	return file, nil
}

// openGCSFile opens a file from Google Cloud Storage with byte-range support
func (p *Processor) openGCSFile(ctx context.Context) (io.Reader, error) {
	// Calculate byte range
	calc := NewChunkCalculator(p.config.InputDataSize, p.config.TotalInstances)
	byteRange, err := calc.Calculate(p.config.InstanceID)
	if err != nil {
		p.errorHandler.HandleError(ErrProcessing, err)
		return nil, fmt.Errorf("calculate byte range: %w", err)
	}

	log.Printf("Opening GCS file: %s (bytes %d-%d)", p.config.InputDataPath, byteRange.StartByte, byteRange.EndByte)

	// If InputDataSize not set, fetch it from GCS
	if p.config.InputDataSize == 0 {
		size, err := GetGCSObjectSize(ctx, p.config.InputDataPath)
		if err != nil {
			p.errorHandler.HandleError(ErrFileNotFound, err)
			return nil, err
		}
		p.config.InputDataSize = size
		log.Printf("GCS file size: %d bytes", p.config.InputDataSize)

		// Recalculate range with actual size
		byteRange, err = calc.Calculate(p.config.InstanceID)
		if err != nil {
			return nil, fmt.Errorf("recalculate byte range: %w", err)
		}
	}

	// Open GCS range reader
	reader, err := NewGCSRangeReader(ctx, p.config.InputDataPath, byteRange.StartByte, byteRange.EndByte)
	if err != nil {
		p.errorHandler.HandleError(ErrFileNotFound, err)
		return nil, err
	}

	return reader, nil
}

// WriteMetrics writes the metrics to output location (local or GCS)
func (p *Processor) WriteMetrics(ctx context.Context, metrics *ProcessMetrics) error {
	// Check if GCS output
	if IsGCSPath(p.config.OutputBasePath) {
		return p.writeMetricsToGCS(ctx, metrics)
	}

	// Local file output
	outputDir := p.config.OutputBasePath
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		p.errorHandler.HandleError(ErrPermissionDenied, err)
		return fmt.Errorf("cannot create output directory: %w", err)
	}

	// Create output filename
	outputFile := filepath.Join(outputDir, fmt.Sprintf("instance-%d.json", p.config.InstanceID))

	// Convert metrics to JSON
	jsonData, err := metrics.ToJSON()
	if err != nil {
		p.errorHandler.HandleError(ErrProcessing, err)
		return fmt.Errorf("cannot marshal metrics to JSON: %w", err)
	}

	// Write to file
	err = os.WriteFile(outputFile, jsonData, 0644)
	if err != nil {
		p.errorHandler.HandleError(ErrPermissionDenied, err)
		return fmt.Errorf("cannot write metrics file: %w", err)
	}

	log.Printf("Metrics written to: %s", outputFile)
	return nil
}

// writeMetricsToGCS writes metrics to Google Cloud Storage
func (p *Processor) writeMetricsToGCS(ctx context.Context, metrics *ProcessMetrics) error {
	// Construct GCS path: gs://bucket/base_path/instance-{id}.json
	outputPath := fmt.Sprintf("%s/instance-%d.json", strings.TrimSuffix(p.config.OutputBasePath, "/"), p.config.InstanceID)

	log.Printf("Writing metrics to GCS: %s", outputPath)

	// Convert metrics to JSON
	jsonData, err := metrics.ToJSON()
	if err != nil {
		p.errorHandler.HandleError(ErrProcessing, err)
		return fmt.Errorf("cannot marshal metrics to JSON: %w", err)
	}

	// Write to GCS with retry
	err = p.errorHandler.HandleWithRetry(func() error {
		return WriteGCSFile(ctx, outputPath, jsonData)
	}, "Write metrics to GCS")

	if err != nil {
		p.errorHandler.HandleError(ErrNetworkTimeout, err)
		return err
	}

	log.Printf("Metrics written to GCS: %s", outputPath)
	return nil
}
