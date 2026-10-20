package jobs

import (
	"log/slog"
)

// holidaysHistoryDays is how far back school holidays are fetched, to describe
// the observed days.
const holidaysHistoryDays = 400

// SyncHolidays fetches the school holidays.
func (r *Runner) SyncHolidays() {
	ctx, cancel := r.context()
	defer cancel()

	periods, err := r.Holidays.Periods(ctx, r.Now().AddDate(0, 0, -holidaysHistoryDays))
	if err != nil {
		slog.Error("cannot fetch the school holidays", "error", err)
		return
	}
	if err = r.Store.SaveHolidays(ctx, periods); err != nil {
		slog.Error("cannot store the school holidays", "error", err)
		return
	}
	slog.Info("school holidays synchronized", "periods", len(periods))
}
