// adapted from
// https://github.com/HDT3213/rdb/blob/b5e024cd842d14b69fed753024ed443949ac5221/helper/json.go#L1
package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"httpcache/pkg/cache"
	"os"

	"github.com/bytedance/sonic"
	"github.com/hdt3213/rdb/core"
	"github.com/hdt3213/rdb/model"
	"github.com/vmihailenco/msgpack/v5"
)

var jsonEncoder = sonic.ConfigDefault

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

func decodeStream(responseChan <-chan cache.Response) <-chan cache.Response {
	decodedChan := make(chan cache.Response, 1000)
	go func() {
		defer close(decodedChan)
		itemCount := 0
		for response := range responseChan {
			encoding := response.Header.Get("Content-Encoding")

			// If content is gzip encoded, decode it
			if encoding == "gzip" {
				// Check if data actually looks like gzip (starts with magic number 0x1f, 0x8b)
				if len(response.Value) < 2 || response.Value[0] != 0x1f || response.Value[1] != 0x8b {
					fmt.Fprintf(os.Stderr, "Content-Encoding is gzip but data doesn't have gzip magic header (len=%d, first bytes: %x)\n",
						len(response.Value), response.Value[:min(len(response.Value), 10)])
					// Send original response since it's not actually gzip
					continue
				}

				gzipReader, err := gzip.NewReader(bytes.NewReader(response.Value))
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to create gzip reader: %v (data len=%d, first bytes: %x)\n",
						err, len(response.Value), response.Value[:min(len(response.Value), 10)])
					// Send original response on error
					continue
				}

				var decodedContent bytes.Buffer
				_, err = decodedContent.ReadFrom(gzipReader)
				gzipReader.Close()

				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to decompress gzip content: %v\n", err)
					// Send original response on error
					continue
				}

				// Update response with decoded content
				response.Value = decodedContent.Bytes()

				// Remove Content-Encoding header since content is no longer compressed
				response.Header.Del("Content-Encoding")

				// Update Content-Length header if it exists
				if response.Header.Get("Content-Length") != "" {
					response.Header.Set("Content-Length", fmt.Sprintf("%d", len(response.Value)))
				}

				itemCount++
			}

			decodedChan <- response
		}
		fmt.Fprintf(os.Stderr, "Total items decoded: %d\n", itemCount)
	}()
	return decodedChan
}

func writeResponse(jsonFileName string, responseChan <-chan cache.Response) error {
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
	// decodedChan := decodeStream(responseChan)
	// err := writeResponse(jsonFilename, decodedChan)
	err := writeResponse(jsonFilename, responseChan)
	if err != nil {
		return fmt.Errorf("write response failed, %v", err)
	}
	return nil
}
