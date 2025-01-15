package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/gpr3211/boot-s3-course/internal/auth"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}
	fmt.Println(userID)

	const maxMemory = 1 << 30 // 1 GB size limit
	r.ParseMultipartForm(maxMemory)

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", nil)
		return
	}
	assetPath := getAssetPath(mediaType)
	fmt.Println("Asset path is ", assetPath)

	dst, err := os.CreateTemp("", "*"+filepath.Ext(assetPath))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create file on server", err)
		return
	}

	defer os.Remove(dst.Name())
	defer dst.Close()
	// defer is LIFO !!!

	if _, err = io.Copy(dst, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving file", err)
		return
	}

	ar, err := GetVideoAspectRatio(dst.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error getting aspect ratio", err)
		return
	}

	processedPath, err := processVideForFastStart(dst.Name())
	if err != nil {
		log.Printf("Failed to process Video: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Error processing video", err)
		return
	}
	defer os.Remove(processedPath) // Clean up the processed file when done

	// Open the processed file
	processedFile, err := os.Open(processedPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error opening processed file", err)
		fmt.Println("Failed to open file")
		return
	}
	defer processedFile.Close()

	finalPath := SetAspectPrefix(assetPath, ar)
	fmt.Printf("Final path uploaded to S3: %s\n", finalPath)

	s3Object := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Body:        processedFile, // Use the file directly instead of bytes buffer
		Key:         &finalPath,
		ContentType: &mediaType,
	}
	putObj, err := cfg.s3Client.PutObject(context.TODO(), &s3Object)
	if err != nil {
		respondWithError(w, 407, "error uploading to s3", err)
	}
	fmt.Println(putObj)
	// update DB with new video link

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", nil)
		return
	}

	s3Url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, *s3Object.Key)
	fmt.Printf("S3 video url %s \nUpdating DB video item...\n", s3Url)
	video.VideoURL = &s3Url
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error updating DB video item", err)
		return
	}

	// next add object to S3 bucket
	respondWithJSON(w, 200, "success")

	return
}
