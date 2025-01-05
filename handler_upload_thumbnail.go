package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
	spl := strings.Fields(meta)
	out, _ := strings.CutSuffix(spl[0], ";")
	final := strings.Split(out, "/")

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

	thumbURL := fmt.Sprintf("http://localhost:%s/api/assets/%s.%s", cfg.port, videoID.String(), final[0])

	metadata.ThumbnailURL = &thumbURL
	err = cfg.db.UpdateVideo(metadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "internal server err", errors.New("internal server error"))
		return
	}

	// TODO: implement the upload here

	fullPath, _ := os.Getwd()
	assetPath := filepath.Join(fullPath, cfg.assetsRoot, fmt.Sprintf("%s.%s", videoID.String(), final[0]))
	err = writeToFile(finalData, assetPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "internal server err", errors.New("internal server error"))
		return
	}

	respondWithJSON(w, http.StatusOK, metadata)
}

func writeToFile(s []byte, path string) error {
	fd, err := os.Create(path)
	if err != nil {
		fmt.Println("failed opening file", err)
		return err
	}
	defer fd.Close()
	n, err := fd.Write(s)
	if err != nil {

		fmt.Println("failed writing file", err)
		return err
	}
	fmt.Printf("%d Bytes to %s", n, path)
	return nil
}
