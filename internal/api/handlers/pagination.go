package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type pageParams struct {
	Page    int
	PerPage int
}

// parsePage reads ?page and ?per_page from query string with sane defaults.
// Optional first argument overrides the default per_page (default: 20).
func parsePage(c *fiber.Ctx, defaultPerPage ...int) pageParams {
	dp := 20
	if len(defaultPerPage) > 0 && defaultPerPage[0] > 0 {
		dp = defaultPerPage[0]
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", strconv.Itoa(dp)))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 20
	}
	return pageParams{Page: page, PerPage: perPage}
}

// calcTotalPages returns the number of pages needed for total items at perPage each.
func calcTotalPages(total int64, perPage int) int {
	pages := int(total) / perPage
	if int(total)%perPage > 0 {
		pages++
	}
	return pages
}
