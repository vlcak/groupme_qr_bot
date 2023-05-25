package groupme

import (
	"bytes"
	"encoding/json"
	"log"
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
		log.Printf("Can't create request %v\n", err)
		return "", err
	}
	r.Header.Add("Content-Type", "image/png")
	r.Header.Add("X-Access-Token", is.userToken)
	client := &http.Client{}
	response, err := client.Do(r)
	if err != nil {
		log.Printf("Upload image error %v\n", err)
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		log.Printf("Image upload returned unexpected code: %d\n", response.StatusCode)
		return "", err
	}

	imageResponse := ImageResponse{}
	if err := json.NewDecoder(response.Body).Decode(&imageResponse); err != nil {
		log.Fatalf("ERROR: %v\n", err)
		return "", err
	}

	return imageResponse.Payload.URL, nil
}
