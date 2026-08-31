package integration_test

import (
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/agopalakrishnan/teams360/backend/infrastructure/dataprovider"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Data Provider Client", func() {
	Describe("LoadConfig", func() {
		BeforeEach(func() {
			os.Unsetenv("DATA_PROVIDER_BASE_URL")
			os.Unsetenv("DATA_PROVIDER_API_TOKEN")
		})

		Context("when DATA_PROVIDER_BASE_URL is not set", func() {
			It("returns nil", func() {
				Expect(dataprovider.LoadConfig()).To(BeNil())
			})
		})

		Context("when DATA_PROVIDER_BASE_URL is set", func() {
			BeforeEach(func() {
				os.Setenv("DATA_PROVIDER_BASE_URL", "https://provider.example.com")
				os.Setenv("DATA_PROVIDER_API_TOKEN", "secret-token")
			})

			AfterEach(func() {
				os.Unsetenv("DATA_PROVIDER_BASE_URL")
				os.Unsetenv("DATA_PROVIDER_API_TOKEN")
			})

			It("returns a config populated from the environment", func() {
				config := dataprovider.LoadConfig()
				Expect(config).NotTo(BeNil())
				Expect(config.BaseURL).To(Equal("https://provider.example.com"))
				Expect(config.APIToken).To(Equal("secret-token"))
			})
		})

		Context("when DATA_PROVIDER_API_TOKEN is not set", func() {
			BeforeEach(func() {
				os.Setenv("DATA_PROVIDER_BASE_URL", "https://provider.example.com")
			})

			AfterEach(func() {
				os.Unsetenv("DATA_PROVIDER_BASE_URL")
			})

			It("returns a config with an empty token", func() {
				config := dataprovider.LoadConfig()
				Expect(config).NotTo(BeNil())
				Expect(config.APIToken).To(BeEmpty())
			})
		})
	})

	Describe("Client.Do", func() {
		var (
			server         *httptest.Server
			receivedHeader string
			receivedPath   string
		)

		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeader = r.Header.Get("x-api-token")
				receivedPath = r.URL.Path
				w.WriteHeader(http.StatusOK)
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("sends the configured token in the x-api-token header", func() {
			client := dataprovider.NewClient(&dataprovider.Config{
				BaseURL:  server.URL,
				APIToken: "secret-token",
			})

			resp, err := client.Do(http.MethodGet, "/pods", nil)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(receivedHeader).To(Equal("secret-token"))
			Expect(receivedPath).To(Equal("/pods"))
		})

		It("joins the base URL and path correctly regardless of slashes", func() {
			client := dataprovider.NewClient(&dataprovider.Config{
				BaseURL:  server.URL + "/",
				APIToken: "secret-token",
			})

			resp, err := client.Do(http.MethodGet, "/pods", nil)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(receivedPath).To(Equal("/pods"))
		})
	})
})
