// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/agopalakrishnan/teams360/backend/interfaces/api/middleware"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validation Middleware", func() {
	var router *gin.Engine

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
		router = gin.New()
	})

	Describe("Input Sanitization", func() {
		Context("SanitizeString function", func() {
			It("should escape HTML special characters", func() {
				input := "<script>alert('xss')</script>"
				expected := "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"
				Expect(middleware.SanitizeString(input)).To(Equal(expected))
			})

			It("should remove null bytes", func() {
				input := "hello\x00world"
				expected := "helloworld"
				Expect(middleware.SanitizeString(input)).To(Equal(expected))
			})

			It("should trim whitespace", func() {
				input := "   hello world   "
				expected := "hello world"
				Expect(middleware.SanitizeString(input)).To(Equal(expected))
			})
		})

		Context("SanitizeComment function", func() {
			It("should limit comment length to 1000 characters", func() {
				input := ""
				for i := 0; i < 1500; i++ {
					input += "a"
				}
				result := middleware.SanitizeComment(input)
				Expect(len(result)).To(Equal(1000))
			})

			It("should escape HTML in comments", func() {
				input := "This is a <b>bold</b> statement"
				expected := "This is a &lt;b&gt;bold&lt;/b&gt; statement"
				Expect(middleware.SanitizeComment(input)).To(Equal(expected))
			})
		})
	})

	Describe("Validation Patterns", func() {
		Context("IsValidUUID", func() {
			It("should accept valid UUIDs", func() {
				Expect(middleware.IsValidUUID("123e4567-e89b-12d3-a456-426614174000")).To(BeTrue())
				Expect(middleware.IsValidUUID("550e8400-e29b-41d4-a716-446655440000")).To(BeTrue())
			})

			It("should reject invalid UUIDs", func() {
				Expect(middleware.IsValidUUID("not-a-uuid")).To(BeFalse())
				Expect(middleware.IsValidUUID("123e4567-e89b-12d3-a456")).To(BeFalse())
				Expect(middleware.IsValidUUID("")).To(BeFalse())
			})
		})

		Context("IsValidUsername", func() {
			It("should accept valid usernames", func() {
				Expect(middleware.IsValidUsername("john")).To(BeTrue())
				Expect(middleware.IsValidUsername("john_doe")).To(BeTrue())
				Expect(middleware.IsValidUsername("john-doe")).To(BeTrue())
				Expect(middleware.IsValidUsername("JohnDoe123")).To(BeTrue())
			})

			It("should reject invalid usernames", func() {
				Expect(middleware.IsValidUsername("a")).To(BeFalse())        // Too short (1 char)
				Expect(middleware.IsValidUsername("john@doe")).To(BeFalse()) // Invalid char
				Expect(middleware.IsValidUsername("john doe")).To(BeFalse()) // Space
				Expect(middleware.IsValidUsername("")).To(BeFalse())         // Empty
			})
		})

		Context("IsValidEmail", func() {
			It("should accept valid emails", func() {
				Expect(middleware.IsValidEmail("user@example.com")).To(BeTrue())
				Expect(middleware.IsValidEmail("user.name@example.co.uk")).To(BeTrue())
				Expect(middleware.IsValidEmail("user+tag@example.com")).To(BeTrue())
			})

			It("should reject invalid emails", func() {
				Expect(middleware.IsValidEmail("not-an-email")).To(BeFalse())
				Expect(middleware.IsValidEmail("user@")).To(BeFalse())
				Expect(middleware.IsValidEmail("@example.com")).To(BeFalse())
				Expect(middleware.IsValidEmail("")).To(BeFalse())
			})
		})
	})

	Describe("Rate Limiting", func() {
		Context("RateLimiter", func() {
			It("should allow requests within the limit", func() {
				limiter := middleware.NewRateLimiter(5, time.Minute)

				for i := 0; i < 5; i++ {
					Expect(limiter.Allow("test-client")).To(BeTrue())
				}
			})

			It("should block requests exceeding the limit", func() {
				limiter := middleware.NewRateLimiter(3, time.Minute)

				// First 3 should be allowed
				for i := 0; i < 3; i++ {
					Expect(limiter.Allow("test-client")).To(BeTrue())
				}

				// 4th should be blocked
				Expect(limiter.Allow("test-client")).To(BeFalse())
			})

			It("should track different clients separately", func() {
				limiter := middleware.NewRateLimiter(2, time.Minute)

				Expect(limiter.Allow("client-a")).To(BeTrue())
				Expect(limiter.Allow("client-a")).To(BeTrue())
				Expect(limiter.Allow("client-a")).To(BeFalse())

				// Different client should still be allowed
				Expect(limiter.Allow("client-b")).To(BeTrue())
			})
		})

		Context("RateLimitMiddleware", func() {
			It("should return 429 when rate limit exceeded", func() {
				limiter := middleware.NewRateLimiter(1, time.Minute)
				router.Use(middleware.RateLimitMiddleware(limiter))
				router.GET("/test", func(c *gin.Context) {
					c.JSON(200, gin.H{"status": "ok"})
				})

				// First request should succeed
				req1, _ := http.NewRequest("GET", "/test", nil)
				req1.RemoteAddr = "192.168.1.1:1234"
				w1 := httptest.NewRecorder()
				router.ServeHTTP(w1, req1)
				Expect(w1.Code).To(Equal(http.StatusOK))

				// Second request should be rate limited
				req2, _ := http.NewRequest("GET", "/test", nil)
				req2.RemoteAddr = "192.168.1.1:1234"
				w2 := httptest.NewRecorder()
				router.ServeHTTP(w2, req2)
				Expect(w2.Code).To(Equal(http.StatusTooManyRequests))
			})
		})
	})

	Describe("Content Type Validation", func() {
		BeforeEach(func() {
			router.Use(middleware.ContentTypeValidator())
			router.POST("/test", func(c *gin.Context) {
				c.JSON(200, gin.H{"status": "ok"})
			})
		})

		It("should accept application/json content type", func() {
			req, _ := http.NewRequest("POST", "/test", bytes.NewBuffer([]byte("{}")))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("should accept application/json with charset", func() {
			req, _ := http.NewRequest("POST", "/test", bytes.NewBuffer([]byte("{}")))
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("should reject non-JSON content types", func() {
			req, _ := http.NewRequest("POST", "/test", bytes.NewBuffer([]byte("<xml></xml>")))
			req.Header.Set("Content-Type", "application/xml")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusUnsupportedMediaType))
		})

		It("should allow requests without content-type header", func() {
			req, _ := http.NewRequest("POST", "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("Max Body Size", func() {
		BeforeEach(func() {
			router.Use(middleware.MaxBodySizeMiddleware(100)) // 100 bytes max
			router.POST("/test", func(c *gin.Context) {
				var body map[string]interface{}
				if err := c.ShouldBindJSON(&body); err != nil {
					dto.RespondError(c, http.StatusBadRequest, "Request body too large")
					return
				}
				c.JSON(200, gin.H{"status": "ok"})
			})
		})

		It("should accept requests within size limit", func() {
			body := map[string]string{"key": "value"}
			jsonData, _ := json.Marshal(body)
			req, _ := http.NewRequest("POST", "/test", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("should reject requests exceeding size limit", func() {
			// Create a body larger than 100 bytes
			largeBody := map[string]string{
				"key": "this is a very long value that will exceed the 100 byte limit set in the middleware configuration",
			}
			jsonData, _ := json.Marshal(largeBody)
			req, _ := http.NewRequest("POST", "/test", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("Path Parameter Validation", func() {
		BeforeEach(func() {
			router.GET("/users/:userId", func(c *gin.Context) {
				userID, ok := middleware.ValidatePathParam(c, "userId", "id")
				if !ok {
					return
				}
				c.JSON(200, gin.H{"userId": userID})
			})

			router.GET("/uuids/:id", func(c *gin.Context) {
				id, ok := middleware.ValidatePathParam(c, "id", "uuid")
				if !ok {
					return
				}
				c.JSON(200, gin.H{"id": id})
			})
		})

		It("should accept valid path parameters", func() {
			req, _ := http.NewRequest("GET", "/users/user123", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("should reject invalid UUID format", func() {
			req, _ := http.NewRequest("GET", "/uuids/not-a-uuid", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))

			var response map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &response)
			Expect(response["error"]).To(ContainSubstring("UUID"))
		})

		It("should accept valid UUID format", func() {
			req, _ := http.NewRequest("GET", "/uuids/123e4567-e89b-12d3-a456-426614174000", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})
	})
})
