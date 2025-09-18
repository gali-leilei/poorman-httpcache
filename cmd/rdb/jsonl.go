// adapted from
// https://github.com/HDT3213/rdb/blob/b5e024cd842d14b69fed753024ed443949ac5221/helper/json.go#L1
package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"httpcache/pkg/cache"
	"net/http"
	"os"

	"github.com/bytedance/sonic"
	"github.com/hdt3213/rdb/core"
	"github.com/hdt3213/rdb/model"
	"github.com/vmihailenco/msgpack/v5"
)

var jsonEncoder = sonic.ConfigDefault

type Response struct {
	Value  string
	Header http.Header
}

// redisObjectChan reads rdb file and returns a channel of RedisObject
func redisObjectStream(rdbFileName string) <-chan model.RedisObject {
	redisObjectBuffer := make(chan model.RedisObject, 1000)
	go func() {
		defer close(redisObjectBuffer)
		redisFile, err := os.Open(rdbFileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening RDB file %s: %v\n", rdbFileName, err)
			return
		}
		defer func() {
			_ = redisFile.Close()
		}()

		dec := core.NewDecoder(redisFile)
		itemCount := 0

		_ = dec.Parse(func(object model.RedisObject) bool {
			itemCount++
			redisObjectBuffer <- object

			// Log progress every 10000 items
			if itemCount%10000 == 0 {
				fmt.Fprintf(os.Stderr, "Processed %d items...\n", itemCount)
			}
			return true
		})

		fmt.Fprintf(os.Stderr, "Total items processed from RDB: %d\n", itemCount)
	}()

	return redisObjectBuffer
}

func filterStream(redisObjectChan <-chan model.RedisObject) <-chan model.StringObject {
	filteredObjectChan := make(chan model.StringObject, 1000)
	go func() {
		defer close(filteredObjectChan)
		itemCount := 0
		for object := range redisObjectChan {
			if object.GetType() == model.StringType {
				if strObject, ok := object.(*model.StringObject); ok {
					filteredObjectChan <- *strObject
					itemCount++
				}
			}
		}
		fmt.Fprintf(os.Stderr, "Total items filtered: %d\n", itemCount)
	}()

	return filteredObjectChan
}

func transformStream(redisObjectChan <-chan model.StringObject) <-chan cache.Response {
	responseChan := make(chan cache.Response, 1000)
	go func() {
		defer close(responseChan)
		itemCount := 0
		for object := range redisObjectChan {
			var payload []byte
			// first layer use msgpack
			err := msgpack.Unmarshal(object.Value, &payload)
			if err != nil {
				fmt.Fprintf(os.Stderr, "msgpack.Unmarshal(object.Value, &payload), %v\n", err)
				return
			}

			// second layer use gob
			response, err := cache.BytesToResponse(payload)
			if err != nil {
				fmt.Fprintf(os.Stderr, "cache.BytesToResponse(payload), %v\n", err)
				return
			}

			responseChan <- response
			itemCount++
		}
		fmt.Fprintf(os.Stderr, "Total items transformed: %d\n", itemCount)
	}()

	return responseChan
}

func decodeStream(responseChan <-chan cache.Response) <-chan Response {
	decodedChan := make(chan Response, 1000)
	go func() {
		defer close(decodedChan)
		itemCount := 0
		wrongCount := 0
		notGzipCount := 0

		// Count per encoding and content type
		encodingCounts := make(map[string]int)
		contentTypeCounts := make(map[string]int)

		for response := range responseChan {
			encoding := response.Header.Get("Content-Encoding")
			contentType := response.Header.Get("Content-Type")

			// Track counts for each encoding and content type
			if encoding == "" {
				encoding = "(none)"
			}
			if contentType == "" {
				contentType = "(none)"
			}
			encodingCounts[encoding]++
			contentTypeCounts[contentType]++

			decodedResponse := Response{
				Header: response.Header,
				Value:  string(response.Value),
			}
			// If content is gzip encoded, decode it
			if encoding == "gzip" && contentType == "text/plain" {
				decodedValue, err := decodeGzip(response.Value)
				if err != nil {
					// fmt.Fprintf(os.Stderr, "decodeBase64Gzip(response.Value), %v\n", err)
					wrongCount++
					continue
				} else {
					itemCount++
				}
				response.Header.Del("Content-Encoding")
				if response.Header.Get("Content-Length") != "" {
					response.Header.Set("Content-Length", fmt.Sprintf("%d", len(response.Value)))
				}

				decodedResponse.Value = string(decodedValue)
			} else {
				notGzipCount++
			}

			decodedChan <- decodedResponse
		}
		fmt.Fprintf(os.Stderr, "Total items decoded: %d\n", itemCount)
		fmt.Fprintf(os.Stderr, "Total items wrong decoded: %d\n", wrongCount)
		fmt.Fprintf(os.Stderr, "Total items not gzip decoded: %d\n", notGzipCount)

		// Print encoding counts
		fmt.Fprintf(os.Stderr, "\nContent-Encoding distribution:\n")
		for encoding, count := range encodingCounts {
			fmt.Fprintf(os.Stderr, "  %s: %d\n", encoding, count)
		}

		// Print content type counts
		fmt.Fprintf(os.Stderr, "\nContent-Type distribution:\n")
		for contentType, count := range contentTypeCounts {
			fmt.Fprintf(os.Stderr, "  %s: %d\n", contentType, count)
		}
	}()
	return decodedChan
}

func writeResponse(jsonFileName string, responseChan <-chan Response) error {
	jsonFile, err := os.Create(jsonFileName)
	if err != nil {
		return fmt.Errorf("create json %s failed, %v", jsonFileName, err)
	}
	defer func() {
		_ = jsonFile.Close()
	}()

	var errs []error
	itemCount := 0
	for response := range responseChan {
		data, err := jsonEncoder.Marshal(response)
		if err != nil {
			errs = append(errs, fmt.Errorf("jsonEncoder.Marshal(response), %v", err))
			continue
		}
		_, err = jsonFile.Write(data)
		if err != nil {
			errs = append(errs, fmt.Errorf("jsonFile.Write(data), %v", err))
			continue
		}
		_, err = jsonFile.WriteString("\n")
		if err != nil {
			errs = append(errs, fmt.Errorf("jsonFile.WriteString(\"\n\"), %v", err))
			continue
		}
		itemCount++
	}
	if len(errs) > 0 {
		return fmt.Errorf("write response failed, %v", errors.Join(errs...))
	}
	fmt.Fprintf(os.Stderr, "Total items written: %d\n", itemCount)
	return nil
}

// decodeBase64Gzip decodes a base64-encoded gzipped string
// equivalent to: base64 -d -i value.txt | gunzip
func decodeGzip(gzippedData []byte) ([]byte, error) {
	// Step 2: Gzip decompress
	reader, err := gzip.NewReader(bytes.NewReader(gzippedData))
	if err != nil {
		return nil, fmt.Errorf("gzip reader creation failed: %v", err)
	}
	defer reader.Close()

	var decompressed bytes.Buffer
	_, err = decompressed.ReadFrom(reader)
	if err != nil {
		return nil, fmt.Errorf("gzip decompression failed: %v", err)
	}

	return decompressed.Bytes(), nil
}

// ToJSONLine reads rdb file and converts to json file
func ToJSONLine(rdbFilename string, jsonFilename string, encoding string) error {
	if rdbFilename == "" {
		return errors.New("rdb filename is required")
	}
	if jsonFilename == "" {
		return errors.New("json filename is required")
	}
	if encoding == "" {
		return errors.New("encoding is required")
	}

	redisObjectChan := redisObjectStream(rdbFilename)
	filteredObjectChan := filterStream(redisObjectChan)
	responseChan := transformStream(filteredObjectChan)
	decodedChan := decodeStream(responseChan)
	err := writeResponse(jsonFilename, decodedChan)
	// err := writeResponse(jsonFilename, responseChan)
	if err != nil {
		return fmt.Errorf("write response failed, %v", err)
	}
	return nil
}
