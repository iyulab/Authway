package handler

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// DocsHandler handles documentation file operations
type DocsHandler struct {
	docsPath string
	logger   *zap.Logger
}

// NewDocsHandler creates a new docs handler
func NewDocsHandler(logger *zap.Logger) *DocsHandler {
	// Get docs path from project root
	docsPath := filepath.Join("docs")
	return &DocsHandler{
		docsPath: docsPath,
		logger:   logger,
	}
}

// DocFile represents a documentation file
type DocFile struct {
	Path         string    `json:"path"`
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	IsDir        bool      `json:"is_dir"`
	Children     []DocFile `json:"children,omitempty"`
	Content      string    `json:"content,omitempty"`
	LastModified string    `json:"last_modified"`
}

// ListDocs returns the documentation file tree
// GET /api/v1/docs
func (h *DocsHandler) ListDocs(c *fiber.Ctx) error {
	tree, err := h.buildFileTree(h.docsPath, "")
	if err != nil {
		h.logger.Error("Failed to build docs tree", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to load documentation tree",
		})
	}

	return c.JSON(fiber.Map{
		"data": tree,
	})
}

// GetDoc returns a specific documentation file content
// GET /api/v1/docs/*
func (h *DocsHandler) GetDoc(c *fiber.Ctx) error {
	// Get the path after /api/v1/docs/
	docPath := c.Params("*")

	// Prevent directory traversal
	if strings.Contains(docPath, "..") {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid path",
		})
	}

	fullPath := filepath.Join(h.docsPath, docPath)

	// Check if file exists
	stat, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return c.Status(404).JSON(fiber.Map{
				"error": "Document not found",
			})
		}
		h.logger.Error("Failed to stat file", zap.Error(err), zap.String("path", fullPath))
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to access document",
		})
	}

	// If directory, return file list
	if stat.IsDir() {
		tree, err := h.buildFileTree(fullPath, docPath)
		if err != nil {
			h.logger.Error("Failed to build tree", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to load directory",
			})
		}
		return c.JSON(fiber.Map{
			"data": tree,
		})
	}

	// Read file content
	content, err := os.ReadFile(fullPath)
	if err != nil {
		h.logger.Error("Failed to read file", zap.Error(err), zap.String("path", fullPath))
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to read document",
		})
	}

	return c.JSON(fiber.Map{
		"data": DocFile{
			Path:         docPath,
			Name:         filepath.Base(docPath),
			Size:         stat.Size(),
			IsDir:        false,
			Content:      string(content),
			LastModified: stat.ModTime().Format("2006-01-02 15:04:05"),
		},
	})
}

// SearchDocs searches for documentation files
// GET /api/v1/docs/search?q=query
func (h *DocsHandler) SearchDocs(c *fiber.Ctx) error {
	query := c.Query("q")
	if query == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Search query is required",
		})
	}

	results := []DocFile{}
	err := filepath.Walk(h.docsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only search .md files
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}

		// Get relative path
		relPath, _ := filepath.Rel(h.docsPath, path)

		// Search in filename
		if strings.Contains(strings.ToLower(info.Name()), strings.ToLower(query)) {
			results = append(results, DocFile{
				Path:         relPath,
				Name:         info.Name(),
				Size:         info.Size(),
				IsDir:        false,
				LastModified: info.ModTime().Format("2006-01-02 15:04:05"),
			})
			return nil
		}

		// Search in content
		content, err := os.ReadFile(path)
		if err == nil && strings.Contains(strings.ToLower(string(content)), strings.ToLower(query)) {
			results = append(results, DocFile{
				Path:         relPath,
				Name:         info.Name(),
				Size:         info.Size(),
				IsDir:        false,
				LastModified: info.ModTime().Format("2006-01-02 15:04:05"),
			})
		}

		return nil
	})

	if err != nil {
		h.logger.Error("Search failed", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error": "Search failed",
		})
	}

	return c.JSON(fiber.Map{
		"data":  results,
		"count": len(results),
	})
}

// UpdateDoc updates a documentation file (admin only)
// PUT /api/v1/docs/*
func (h *DocsHandler) UpdateDoc(c *fiber.Ctx) error {
	docPath := c.Params("*")

	if strings.Contains(docPath, "..") {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid path",
		})
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	fullPath := filepath.Join(h.docsPath, docPath)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		h.logger.Error("Failed to create directory", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create directory",
		})
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(body.Content), 0644); err != nil {
		h.logger.Error("Failed to write file", zap.Error(err), zap.String("path", fullPath))
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to save document",
		})
	}

	h.logger.Info("Document updated", zap.String("path", docPath))

	return c.JSON(fiber.Map{
		"message": "Document saved successfully",
		"path":    docPath,
	})
}

// DeleteDoc deletes a documentation file (admin only)
// DELETE /api/v1/docs/*
func (h *DocsHandler) DeleteDoc(c *fiber.Ctx) error {
	docPath := c.Params("*")

	if strings.Contains(docPath, "..") {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid path",
		})
	}

	fullPath := filepath.Join(h.docsPath, docPath)

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return c.Status(404).JSON(fiber.Map{
				"error": "Document not found",
			})
		}
		h.logger.Error("Failed to delete file", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to delete document",
		})
	}

	h.logger.Info("Document deleted", zap.String("path", docPath))

	return c.JSON(fiber.Map{
		"message": "Document deleted successfully",
	})
}

// UploadDoc uploads a new documentation file (admin only)
// POST /api/v1/docs/upload
func (h *DocsHandler) UploadDoc(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "No file provided",
		})
	}

	// Get target path from form
	targetPath := c.FormValue("path")
	if targetPath == "" {
		targetPath = file.Filename
	}

	if strings.Contains(targetPath, "..") {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid path",
		})
	}

	fullPath := filepath.Join(h.docsPath, targetPath)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		h.logger.Error("Failed to create directory", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create directory",
		})
	}

	// Save file
	if err := c.SaveFile(file, fullPath); err != nil {
		h.logger.Error("Failed to save file", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to upload file",
		})
	}

	h.logger.Info("Document uploaded", zap.String("path", targetPath))

	return c.JSON(fiber.Map{
		"message": "File uploaded successfully",
		"path":    targetPath,
	})
}

// DownloadDoc downloads a documentation file
// GET /api/v1/docs/download/*
func (h *DocsHandler) DownloadDoc(c *fiber.Ctx) error {
	docPath := c.Params("*")

	if strings.Contains(docPath, "..") {
		return c.Status(400).SendString("Invalid path")
	}

	fullPath := filepath.Join(h.docsPath, docPath)

	// Check if file exists
	if _, err := os.Stat(fullPath); err != nil {
		if os.IsNotExist(err) {
			return c.Status(404).SendString("File not found")
		}
		return c.Status(500).SendString("Failed to access file")
	}

	// Send file
	return c.SendFile(fullPath)
}

// buildFileTree recursively builds the file tree structure
func (h *DocsHandler) buildFileTree(dirPath string, relPath string) ([]DocFile, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var files []DocFile
	for _, entry := range entries {
		// Skip hidden files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Build relative path
		itemRelPath := filepath.Join(relPath, entry.Name())

		docFile := DocFile{
			Path:         filepath.ToSlash(itemRelPath),
			Name:         entry.Name(),
			Size:         info.Size(),
			IsDir:        entry.IsDir(),
			LastModified: info.ModTime().Format("2006-01-02 15:04:05"),
		}

		// Recursively get children for directories
		if entry.IsDir() {
			children, err := h.buildFileTree(filepath.Join(dirPath, entry.Name()), itemRelPath)
			if err == nil {
				docFile.Children = children
			}
		}

		files = append(files, docFile)
	}

	return files, nil
}
