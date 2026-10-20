// Package api exposes the forecasts over HTTP, along with the web interface.
package api

import (
	"context"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/romitou/previsix/database/models"
	"github.com/romitou/previsix/dates"
	"github.com/romitou/previsix/forecast"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Store is the subset of the database used by the API.
type Store interface {
	Ping(ctx context.Context) error
	Days(ctx context.Context, from string, to string) ([]models.DayCalendar, error)
	Day(ctx context.Context, date string) ([]models.DayCalendar, error)
	Forecasts(ctx context.Context, calendarID bson.ObjectID, limit int64) ([]models.DayForecast, error)
	LatestSnapshot(ctx context.Context) (*models.WaitSnapshot, error)
	ObservedSlots(ctx context.Context) ([]models.DayCalendar, error)
}

const (
	defaultRangeDays = 120
	maxRangeDays     = 400
	historyLimit     = 50
)

type handler struct {
	store Store
	now   func() time.Time
}

// NewRouter builds the HTTP router. static holds the web interface.
func NewRouter(store Store, static fs.FS, now func() time.Time) *gin.Engine {
	h := handler{store: store, now: now}

	router := gin.New()
	router.Use(gin.Recovery(), requestLogger())
	_ = router.SetTrustedProxies(nil)

	group := router.Group("/api")
	group.GET("/health", h.health)
	group.GET("/days", h.days)
	group.GET("/days/:date", h.day)
	group.GET("/live", h.live)
	group.GET("/model", h.model)

	if static != nil {
		router.NoRoute(gin.WrapH(http.FileServerFS(static)))
	}
	return router
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Debug("http request", "method", c.Request.Method, "path", c.Request.URL.Path, "status", c.Writer.Status(), "duration", time.Since(start))
	}
}

func fail(c *gin.Context, status int, message string, err error) {
	if err != nil {
		slog.Error(message, "path", c.Request.URL.Path, "error", err)
	}
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}

func (h handler) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if err := h.store.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "database": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// days lists the slots between ?from and ?to (YYYY-MM-DD, inclusive).
func (h handler) days(c *gin.Context) {
	today := dates.DayKey(h.now())
	from := c.DefaultQuery("from", today)
	to := c.DefaultQuery("to", dates.AddDays(from, defaultRangeDays))

	start, err := dates.ParseDay(from)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid from date, expected YYYY-MM-DD", nil)
		return
	}
	end, err := dates.ParseDay(to)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid to date, expected YYYY-MM-DD", nil)
		return
	}
	if end.Before(start) || end.Sub(start) > maxRangeDays*24*time.Hour {
		fail(c, http.StatusBadRequest, "the range must be positive and shorter than 400 days", nil)
		return
	}

	days, err := h.store.Days(c.Request.Context(), from, to)
	if err != nil {
		fail(c, http.StatusInternalServerError, "cannot load the calendar", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from, "to": to, "days": days})
}

type slotDetail struct {
	models.DayCalendar
	History []models.DayForecast `json:"history"`
}

// day returns the slots of a day with their forecast history.
func (h handler) day(c *gin.Context) {
	date := c.Param("date")
	if _, err := dates.ParseDay(date); err != nil {
		fail(c, http.StatusBadRequest, "invalid date, expected YYYY-MM-DD", nil)
		return
	}

	slots, err := h.store.Day(c.Request.Context(), date)
	if err != nil {
		fail(c, http.StatusInternalServerError, "cannot load the day", err)
		return
	}
	if len(slots) == 0 {
		fail(c, http.StatusNotFound, "the park is not open on this day", nil)
		return
	}

	details := make([]slotDetail, 0, len(slots))
	for _, slot := range slots {
		history, err := h.store.Forecasts(c.Request.Context(), slot.ID, historyLimit)
		if err != nil {
			fail(c, http.StatusInternalServerError, "cannot load the forecasts", err)
			return
		}
		details = append(details, slotDetail{DayCalendar: slot, History: history})
	}
	c.JSON(http.StatusOK, gin.H{"date": date, "slots": details})
}

func (h handler) live(c *gin.Context) {
	snapshot, err := h.store.LatestSnapshot(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "cannot load the wait times", err)
		return
	}
	if snapshot == nil {
		fail(c, http.StatusNotFound, "no wait times collected yet", nil)
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

type accuracy struct {
	// Amount of slots having both a forecast made before the opening and an observation.
	Slots int `json:"slots"`
	// Mean absolute error of the average wait time, in minutes.
	MAE float64 `json:"mae"`
	// Mean error (forecast - observed), in minutes: positive when overestimating.
	Bias float64 `json:"bias"`
}

func (h handler) model(c *gin.Context) {
	observed, err := h.store.ObservedSlots(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, "cannot load the observations", err)
		return
	}

	var stats accuracy
	for _, slot := range observed {
		if slot.Observed.ForecastAvgWait == nil {
			continue
		}
		difference := *slot.Observed.ForecastAvgWait - slot.Observed.AvgWait
		stats.Slots++
		stats.MAE += math.Abs(difference)
		stats.Bias += difference
	}
	if stats.Slots > 0 {
		stats.MAE = math.Round(stats.MAE/float64(stats.Slots)*100) / 100
		stats.Bias = math.Round(stats.Bias/float64(stats.Slots)*100) / 100
	}

	c.JSON(http.StatusOK, gin.H{
		"model":         forecast.Current(),
		"features":      forecast.FeatureNames,
		"prior":         forecast.PriorCoefficients,
		"observedSlots": len(observed),
		"accuracy":      stats,
	})
}
