package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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

	const maxMemory = 10 << 20

	r.ParseMultipartForm(maxMemory)
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()
	fmt.Println(header)

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	meta := r.Header.Get("Content-Type")

	// fetch metadata from DB
	metadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, 405, "bad header", err)
		return
	}
	// check if user is the creator of video
	if metadata.CreateVideoParams.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "no access", errors.New("Unauthorized attempt"))
		return
	}
	finalData, err := io.ReadAll(file)
	if err != nil {

		respondWithError(w, http.StatusInternalServerError, "internal server error", errors.New("internal server error"))
		return
	}

	newThumb := thumbnail{
		data:      finalData,
		mediaType: string(meta),
	}

	path := "http://localhost:" + cfg.port + "/api/thumbnails/" + videoID.String()
	metadata.ThumbnailURL = &path

	// TODO: implement the upload here

	encodedImage := base64.StdEncoding.EncodeToString(finalData)
	dataURL := fmt.Sprintf("data:%s;base64,%s", meta, encodedImage)
	metadata.ThumbnailURL = &dataURL
	err = cfg.db.UpdateVideo(metadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "internal server error", errors.New("internal server error"))
		return
	}

	respondWithJSON(w, http.StatusOK, newThumb)
}
