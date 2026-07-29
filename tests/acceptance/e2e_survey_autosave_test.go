// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Survey Autosave", Label("e2e"), func() {
	var (
		page playwright.Page
		ctx  playwright.BrowserContext
	)

	BeforeEach(func() {
		var err error
		ctx, err = browser.NewContext()
		Expect(err).NotTo(HaveOccurred())

		page, err = ctx.NewPage()
		Expect(err).NotTo(HaveOccurred())

		// Clear any existing draft before each test
		_, err = page.Goto(frontendURL + "/login")
		Expect(err).NotTo(HaveOccurred())
		_, err = page.Evaluate(`() => {
			// Clear all survey drafts
			for (let i = localStorage.length - 1; i >= 0; i--) {
				const key = localStorage.key(i);
				if (key && key.startsWith('surveyDraft:')) {
					localStorage.removeItem(key);
				}
			}
		}`)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if page != nil {
			page.Close()
		}
		if ctx != nil {
			ctx.Close()
		}
		// Clean up any sessions submitted by e2e_lead1 during autosave tests to avoid
		// polluting the team lead dashboard tests with unexpected current-period data.
		db.Exec(`DELETE FROM health_check_responses WHERE session_id IN (
			SELECT id FROM health_check_sessions WHERE user_id = 'e2e_lead1'
			AND id NOT LIKE 'e2e_%' AND id NOT LIKE 'demo-%'
		)`)
		db.Exec(`DELETE FROM health_check_sessions WHERE user_id = 'e2e_lead1'
			AND id NOT LIKE 'e2e_%' AND id NOT LIKE 'demo-%'`)
	})

	loginAndGoToSurvey := func() {
		By("Logging in as e2e_lead1 (Team Lead, level-4)")
		_, err := page.Goto(frontendURL + "/login")
		Expect(err).NotTo(HaveOccurred())

		err = page.Locator("input[name='username']").WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(10000),
		})
		Expect(err).NotTo(HaveOccurred())

		err = page.Locator("input[name='username']").Fill("e2e_lead1")
		Expect(err).NotTo(HaveOccurred())
		err = page.Locator("input[name='password']").Fill("demo")
		Expect(err).NotTo(HaveOccurred())
		err = page.Locator("button[type='submit']").Click()
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for redirect to dashboard page")
		err = page.WaitForURL("**/dashboard", playwright.PageWaitForURLOptions{
			Timeout: playwright.Float(10000),
		})
		Expect(err).NotTo(HaveOccurred())
		dismissOnboardingIfVisible(page)

		By("Navigating to survey")
		_, err = page.Goto(frontendURL + "/survey")
		Expect(err).NotTo(HaveOccurred())

		err = page.WaitForURL("**/survey", playwright.PageWaitForURLOptions{
			Timeout: playwright.Float(5000),
		})
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for survey to load")
		err = page.Locator("text=Mission").WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(10000),
		})
		Expect(err).NotTo(HaveOccurred())
	}

	fillDimension := func(dimensionID string, score int, trend string) {
		scoreSelector := fmt.Sprintf("[data-dimension='%s'][data-score='%d']", dimensionID, score)
		err := page.Locator(scoreSelector).WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(5000),
		})
		Expect(err).NotTo(HaveOccurred())
		err = page.Locator(scoreSelector).Click()
		Expect(err).NotTo(HaveOccurred())

		trendSelector := fmt.Sprintf("[data-dimension='%s'][data-trend='%s']", dimensionID, trend)
		err = page.Locator(trendSelector).WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(5000),
		})
		Expect(err).NotTo(HaveOccurred())
		err = page.Locator(trendSelector).Click()
		Expect(err).NotTo(HaveOccurred())
	}

	clickNext := func() {
		nextButton := page.Locator("button:has-text('Next')")
		err := nextButton.WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(3000),
		})
		Expect(err).NotTo(HaveOccurred())
		err = nextButton.Click()
		Expect(err).NotTo(HaveOccurred())
		time.Sleep(500 * time.Millisecond)
	}

	Describe("Draft save and restore", func() {
		It("should restore draft progress after page reload", func() {
			loginAndGoToSurvey()

			By("Filling first dimension (Mission) with Green score, Improving trend")
			fillDimension("mission", 3, "improving")
			clickNext()

			By("Filling second dimension (Value) with Yellow score, Stable trend")
			fillDimension("value", 2, "stable")

			By("Waiting for autosave debounce")
			time.Sleep(500 * time.Millisecond)

			By("Reloading the page")
			_, err := page.Reload()
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for survey to reload")
			err = page.Locator("text=Delivering Value").WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(10000),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying draft-restored banner is visible")
			banner := page.Locator("[data-testid='draft-restored-banner']")
			Eventually(func() bool {
				visible, _ := banner.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Verifying we're on the second dimension (Value)")
			heading := page.Locator("h2")
			text, err := heading.TextContent()
			Expect(err).NotTo(HaveOccurred())
			Expect(text).To(ContainSubstring("Delivering Value"))

			By("Verifying the score selection is preserved (Yellow)")
			yellowSelected := page.Locator("[data-dimension='value'][data-score='2']")
			className, err := yellowSelected.GetAttribute("class")
			Expect(err).NotTo(HaveOccurred())
			Expect(className).To(ContainSubstring("border-yellow-500"))

			By("Verifying the trend selection is preserved (Stable)")
			stableSelected := page.Locator("[data-dimension='value'][data-trend='stable']")
			stableClass, err := stableSelected.GetAttribute("class")
			Expect(err).NotTo(HaveOccurred())
			Expect(stableClass).To(ContainSubstring("border-blue-500"))

			By("Dismissing the draft banner")
			dismissBtn := banner.Locator("button[aria-label='Dismiss']")
			err = dismissBtn.Click()
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(500 * time.Millisecond)

			visible, _ := banner.IsVisible()
			Expect(visible).To(BeFalse())
		})
	})

	Describe("Comment preserved on score change", func() {
		It("should not clear comment when changing score color", func() {
			loginAndGoToSurvey()

			By("Selecting a score for Mission")
			fillDimension("mission", 3, "improving")

			By("Typing a comment")
			commentBox := page.Locator("textarea[data-dimension='mission']")
			err := commentBox.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(5000),
			})
			Expect(err).NotTo(HaveOccurred())
			err = commentBox.Fill("This is my important comment")
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(300 * time.Millisecond)

			By("Changing score from Green to Red")
			scoreSelector := "[data-dimension='mission'][data-score='1']"
			err = page.Locator(scoreSelector).Click()
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(300 * time.Millisecond)

			By("Verifying comment is still there")
			commentValue, err := commentBox.InputValue()
			Expect(err).NotTo(HaveOccurred())
			Expect(commentValue).To(Equal("This is my important comment"))

			By("Verifying trend is still set to improving")
			trendBtn := page.Locator("[data-dimension='mission'][data-trend='improving']")
			trendClass, err := trendBtn.GetAttribute("class")
			Expect(err).NotTo(HaveOccurred())
			Expect(trendClass).To(ContainSubstring("border-green-500"))
		})
	})

	Describe("Comment character limit", func() {
		It("should show character count and enforce 1000 char limit", func() {
			loginAndGoToSurvey()

			By("Selecting a score for Mission")
			fillDimension("mission", 3, "improving")

			By("Verifying character counter is visible")
			counter := page.Locator("text=/\\/1000/")
			Eventually(func() bool {
				visible, _ := counter.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Typing a long comment near the limit")
			commentBox := page.Locator("textarea[data-dimension='mission']")
			// Type 960 chars to get near the warning threshold
			longText := strings.Repeat("abcdefghij", 96)
			err := commentBox.Fill(longText)
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(300 * time.Millisecond)

			By("Verifying counter shows warning color (960/1000)")
			warningCounter := page.Locator("p.text-red-500:text-matches('960/1000')")
			Eventually(func() bool {
				visible, _ := warningCounter.IsVisible()
				return visible
			}, 3*time.Second, 300*time.Millisecond).Should(BeTrue())

			By("Verifying the maxLength attribute prevents exceeding 1000 chars")
			maxLen, err := commentBox.GetAttribute("maxlength")
			Expect(err).NotTo(HaveOccurred())
			Expect(maxLen).To(Equal("1000"))
		})
	})

	Describe("Draft cleared on submit", func() {
		It("should not show draft banner after completing and submitting survey", func() {
			loginAndGoToSurvey()

			By("Filling all 11 dimensions")
			dimensions := []struct {
				id    string
				score int
				trend string
			}{
				{"mission", 3, "improving"},
				{"value", 2, "stable"},
				{"speed", 1, "declining"},
				{"fun", 2, "stable"},
				{"health", 3, "improving"},
				{"learning", 3, "improving"},
				{"support", 2, "stable"},
				{"pawns", 3, "improving"},
				{"release", 1, "declining"},
				{"process", 2, "stable"},
				{"teamwork", 3, "improving"},
			}

			for i, d := range dimensions {
				fillDimension(d.id, d.score, d.trend)
				if i < len(dimensions)-1 {
					clickNext()
				}
			}

			By("Submitting the survey")
			submitButton := page.Locator("button[type='submit']:has-text('Submit')")
			err := submitButton.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(3000),
			})
			Expect(err).NotTo(HaveOccurred())
			err = submitButton.Click()
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for redirect to home page")
			err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
				Timeout: playwright.Float(10000),
			})
			Expect(err).NotTo(HaveOccurred())
			dismissOnboardingIfVisible(page)

			By("Navigating back to survey")
			surveyBtn := page.Locator("[data-testid='take-survey-btn']")
			err = surveyBtn.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(5000),
			})
			Expect(err).NotTo(HaveOccurred())
			err = surveyBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			err = page.WaitForURL("**/survey", playwright.PageWaitForURLOptions{
				Timeout: playwright.Float(5000),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying survey starts fresh (no draft banner)")
			err = page.Locator("text=Mission").WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(10000),
			})
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(1 * time.Second)
			banner := page.Locator("[data-testid='draft-restored-banner']")
			visible, _ := banner.IsVisible()
			Expect(visible).To(BeFalse())
		})
	})
})
