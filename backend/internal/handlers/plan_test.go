package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidateWeekPlanRuleHandlerAcceptsValidExpression(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/week-plan/rules/validate", strings.NewReader(`{
		"code":"test_rule",
		"name":"测试规则",
		"enabled":true,
		"scope":"meal",
		"rule_kind":"constraint",
		"severity":"hard",
		"relaxable":false,
		"expression":"!has(candidate.protein_sources, \"egg\") || countMeal(\"protein_sources\", \"egg\") == 0",
		"priority":10,
		"message":"同餐蛋类已达到上限"
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ValidateWeekPlanRuleHandler(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestValidateWeekPlanRuleHandlerRejectsBadExpression(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/week-plan/rules/validate", strings.NewReader(`{
		"code":"bad_rule",
		"name":"错误规则",
		"enabled":true,
		"scope":"meal",
		"rule_kind":"constraint",
		"severity":"hard",
		"relaxable":false,
		"expression":"unknown_value > 0",
		"priority":10,
		"message":""
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ValidateWeekPlanRuleHandler(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}
