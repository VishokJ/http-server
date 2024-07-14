package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

var directory string

func main() {
	fmt.Println("Application started...")
	flag.StringVar(&directory, "directory", "", "The directory to read the file from")
	flag.Parse()
	if directory != "" {
		fmt.Printf("Reading from directory: %s", directory)
	}
	l, err := net.Listen("tcp", "0.0.0.0:4221") // listening on the port
	if err != nil {
		fmt.Println("Failed to bind to port 4221")
		os.Exit(1)
	}
	defer l.Close()

	for { // typically web servers are implemented as infinitely running for-loops!
		con, err := l.Accept() // when the client connects, accept the connection - this is a blocking call
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			os.Exit(1)
		}
		fmt.Println("Connection accepted...")
		go handle(con) // multi-threading with the go-routine allows for concurrent connections
	}
}

func handle(con net.Conn) {
	fmt.Println("Handling connection...")
	defer con.Close()

	data := make([]byte, 0)
	buffer := make([]byte, 1024) // buffer is the byte arr of the req, max length of req is 1024 bytes
	n, err := con.Read(buffer)   // also blocking, this reads the request
	if err != nil && err != io.EOF {
		fmt.Println("Error reading:", err)
		return
	}
	data = append(data, buffer[:n]...)

	path := strings.Split(string(data), " ")[1]
	method, headers, body := parseRequest(string(data))
	fmt.Println("Method:", method)
	fmt.Println("Headers:", headers)
	fmt.Println("Body:", body)

	var response string

	switch {
	case path == "/":
		response = createResponse("200 OK", nil, "")
	case strings.HasPrefix(path, "/echo"):
		response = echo(strings.TrimPrefix(path, "/echo/"), headers["accept-encoding"])
	case strings.HasPrefix(path, "/user-agent"):
		response = returnUserAgent(headers["user-agent"])
	case strings.HasPrefix(path, "/files"):
		if method == "GET" {
			response = returnFileIfExists(strings.TrimPrefix(path, "/files/"))
		} else if method == "POST" {
			response = createFile(strings.TrimPrefix(path, "/files/"), []byte(body))
		}
	default:
		response = createResponse("404 Not Found", nil, "")
	}

	_, err = con.Write([]byte(response))
	if err != nil {
		fmt.Println("Error writing: ", err)
	}

	fmt.Println("Response sent: ", response)
}

func parseRequest(s string) (string, map[string]string, string) {
	headers := make(map[string]string)

	parts := strings.SplitN(s, "\r\n\r\n", 2)
	lines := strings.Split(parts[0], "\n")
	method := strings.Split(lines[0], " ")[0]
	for _, line := range lines[1:] { // skip the first line (request line)
		line = strings.TrimSpace(line)
		if line == "" {
			break // last line, done with headers
		}
		split := strings.SplitN(line, ":", 2)
		if len(split) == 2 {
			k := strings.TrimSpace(strings.ToLower(split[0]))
			v := strings.TrimSpace(split[1])
			headers[k] = v
		}
	}
	if len(parts) > 1 {
		return method, headers, parts[1]
	}
	return method, headers, ""
}

func createResponse(status string, headers map[string]string, body string) string {
	resp := "HTTP/1.1 " + status + "\r\n"
	for k, v := range headers {
		resp += k + ": " + v + "\r\n"
	}
	resp += "\r\n"
	resp += body
	return resp
}

func echo(s string, encoding string) string {
	encoding = strings.TrimSpace(strings.ReplaceAll(encoding, " ", ""))
	encodings := strings.Split(encoding, ",")

	headers := map[string]string{
		"Content-Type": "text/plain",
	}

	var body string
	if slices.Contains(encodings, "gzip") {
		headers["Content-Encoding"] = "gzip"
		compressedData, err := compressGzip([]byte(s))
		if err != nil {
			body = s // Fallback to uncompressed data
		} else {
			body = string(compressedData) // use compressed data without base64 encoding
		}
	} else {
		body = s
	}

	headers["Content-Length"] = strconv.Itoa(len(body))
	return createResponse("200 OK", headers, body)
}

func compressGzip(data []byte) ([]byte, error) {
	var b bytes.Buffer
	w, err := gzip.NewWriterLevel(&b, gzip.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip writer: %w", err)
	}

	_, err = w.Write(data)
	if err != nil {
		return nil, fmt.Errorf("failed to write data to gzip writer: %w", err)
	}

	err = w.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return b.Bytes(), nil
}

func returnUserAgent(ua string) string {
	headers := map[string]string{"Content-Type": "text/plain",
		"Content-Length": strconv.Itoa(utf8.RuneCountInString(ua))}
	return createResponse("200 OK", headers, ua)
}

func returnFileIfExists(file string) string {
	fullFile := filepath.Join(directory, file)
	data, err := os.ReadFile(fullFile)
	if err != nil {
		fmt.Println("File does not exist: ", file)
		return createResponse("404 Not Found", nil, "")
	} else {
		fmt.Println("File found: ", file)
		headers := map[string]string{"Content-Type": "application/octet-stream",
			"Content-Length": strconv.Itoa(utf8.RuneCountInString(string(data)))}
		return createResponse("200 OK", headers, string(data))
	}
}

func createFile(file string, body []byte) string {
	fmt.Println("CREATING FILE!")
	fullFile := filepath.Join(directory, file)
	f, err := os.Create(fullFile)
	if err != nil {
		fmt.Println("Error creating file: ", file, err.Error())
		return createResponse("400 Bad Request", nil, "")
	} else {
		fmt.Printf("Created file: %s\n", file)
		n, err := f.Write(body)
		if err != nil {
			return createResponse("400 Bad Request", nil, "")
		} else {
			fmt.Printf("Wrote %d bytes\n", n)
			return createResponse("201 Created", nil, "")
		}
	}
}
