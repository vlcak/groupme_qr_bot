package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	ImageURL = "https://image.groupme.com/pictures"
)

type ImageResponse struct {
	Payload struct {
		URL        string `json:"url"`
		PictureURL string `json:"picture_url"`
	} `json:"payload"`
}

func NewImageService(userToken string) *ImageService {
	sender := &ImageService{
		userToken: userToken,
	}
	return sender
}

type ImageService struct {
	userToken string
}

func (is *ImageService) Upload(image []byte) (string, error) {
	body := bytes.NewBuffer(image)
	r, err := http.NewRequest("POST", ImageURL, body)
	if err != nil {
		fmt.Printf("Can't create request %v\n", err)
		return "", err
	}
	r.Header.Add("Content-Type", "image/png")
	r.Header.Add("X-Access-Token", is.userToken)
	client := &http.Client{}
	response, err := client.Do(r)
	if err != nil {
		fmt.Printf("Upload image error %v\n", err)
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		fmt.Printf("Image upload returned unexpected code: %d\n", response.StatusCode)
		return "", err
	}

	imageResponse := ImageResponse{}
	if err := json.NewDecoder(response.Body).Decode(&imageResponse); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return "", err
	}

	return imageResponse.Payload.URL, nil
}
