"use strict";

const TZ = "Europe/Paris";
const MONTHS_SHOWN = 4;

const LEVELS = [
  { max: 0.2, label: "Très calme" },
  { max: 0.35, label: "Calme" },
  { max: 0.5, label: "Modéré" },
  { max: 0.65, label: "Chargé" },
  { max: 0.8, label: "Très chargé" },
  { max: Infinity, label: "Bondé" },
];

const WEEKDAYS = ["lun", "mar", "mer", "jeu", "ven", "sam", "dim"];

const escapeHTML = (value) =>
  String(value).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);

const levelOf = (score) => LEVELS.findIndex((level) => score < level.max) + 1;

const timeFormat = new Intl.DateTimeFormat("fr-FR", { timeZone: TZ, hour: "2-digit", minute: "2-digit" });
const dateTimeFormat = new Intl.DateTimeFormat("fr-FR", { timeZone: TZ, day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
const longDateFormat = new Intl.DateTimeFormat("fr-FR", { timeZone: "UTC", weekday: "long", day: "numeric", month: "long", year: "numeric" });
const monthFormat = new Intl.DateTimeFormat("fr-FR", { timeZone: "UTC", month: "long", year: "numeric" });

const formatTime = (iso) => timeFormat.format(new Date(iso));
const formatHour = (iso) => formatTime(iso).slice(0, 2) + "h";
const minutes = (value) => `${Math.round(value)} min`;

// Days are handled as YYYY-MM-DD strings, and as UTC midnights for arithmetics.
const parseDay = (day) => new Date(`${day}T00:00:00Z`);
const formatDay = (date) => date.toISOString().slice(0, 10);
const todayInParis = () => new Intl.DateTimeFormat("en-CA", { timeZone: TZ }).format(new Date());

async function getJSON(path) {
  const response = await fetch(path, { headers: { Accept: "application/json" } });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    const error = new Error(body.error || response.statusText);
    error.status = response.status;
    throw error;
  }
  return response.json();
}

function renderLegend() {
  document.getElementById("legend").innerHTML = LEVELS.map(
    (level, index) => `<li><span class="swatch level-${index + 1}"></span>${level.label}</li>`
  ).join("") + `<li>🎃 soirée événement</li>`;
}

// The figures shown for a slot: what was observed when available, the latest forecast otherwise.
function slotFigures(slot) {
  if (slot.observed) return { score: slot.observed.crowdScore, avgWait: slot.observed.avgWait, observed: true };
  if (slot.latestForecast) return { score: slot.latestForecast.crowdScore, avgWait: slot.latestForecast.avgWait, observed: false };
  return null;
}

let selectedDay = null;

async function renderCalendar() {
  const container = document.getElementById("calendar");
  const today = todayInParis();
  const first = parseDay(today);
  first.setUTCDate(1);
  const last = new Date(first);
  last.setUTCMonth(last.getUTCMonth() + MONTHS_SHOWN);
  last.setUTCDate(0);

  let days;
  try {
    ({ days } = await getJSON(`/api/days?from=${formatDay(first)}&to=${formatDay(last)}`));
  } catch (error) {
    container.innerHTML = `<p class="muted">Impossible de charger le calendrier : ${escapeHTML(error.message)}</p>`;
    return;
  }

  const byDate = new Map();
  for (const slot of days) {
    if (!byDate.has(slot.date)) byDate.set(slot.date, []);
    byDate.get(slot.date).push(slot);
  }

  let html = "";
  for (let month = new Date(first); month <= last; month.setUTCMonth(month.getUTCMonth() + 1)) {
    html += `<div class="month"><h3>${monthFormat.format(month)}</h3><div class="grid">`;
    html += WEEKDAYS.map((day) => `<span class="weekday">${day}</span>`).join("");
    const offset = (month.getUTCDay() + 6) % 7;
    html += `<span></span>`.repeat(offset);

    const cursor = new Date(month);
    while (cursor.getUTCMonth() === month.getUTCMonth()) {
      const date = formatDay(cursor);
      const slots = byDate.get(date) || [];
      const number = cursor.getUTCDate();
      if (slots.length === 0) {
        html += `<span class="day" title="Parc fermé">${number}</span>`;
      } else {
        const main = slots.find((slot) => slot.dayType === "NORMAL") || slots[0];
        const figures = slotFigures(main);
        const event = slots.some((slot) => slot.dayType === "EVENT");
        const classes = ["day", figures ? `level-${levelOf(figures.score)}` : "pending"];
        if (date < today) classes.push("past");
        if (date === selectedDay) classes.push("selected");
        const title = figures
          ? `${LEVELS[levelOf(figures.score) - 1].label}, ${minutes(figures.avgWait)} d'attente moyenne${figures.observed ? " (observé)" : ""}`
          : "Prévision en cours";
        html += `<button class="${classes.join(" ")}" data-date="${date}" title="${escapeHTML(title)}">${number}${event ? '<span class="event">🎃</span>' : ""}</button>`;
      }
      cursor.setUTCDate(cursor.getUTCDate() + 1);
    }
    html += `</div></div>`;
  }
  container.innerHTML = html;
}

function featureChips(features) {
  if (!features) return "";
  const chips = [];
  const zones = ["A", "B", "C"].filter((zone) => features[`zone${zone}`]);
  if (zones.length) chips.push(`🎒 Vacances zone${zones.length > 1 ? "s" : ""} ${zones.join(", ")}`);
  if (features.publicHoliday) chips.push("🇫🇷 Jour férié");
  if (features.bridge) chips.push("🌉 Pont");
  if (features.nightEvent) chips.push("🎃 Soirée événement");
  else if (features.eventDay) chips.push("🎃 Soirée événement ce jour");
  if (features.weather) {
    const weather = features.weather;
    chips.push(`🌡️ ${Math.round(weather.temperatureMax)} °C`);
    chips.push(`${weather.precipitationProbability >= 50 ? "🌧️" : "☀️"} Pluie ${Math.round(weather.precipitationProbability)} %`);
  } else {
    chips.push("☁️ Météo pas encore connue");
  }
  return `<ul class="chips">${chips.map((chip) => `<li>${escapeHTML(chip)}</li>`).join("")}</ul>`;
}

function hourlyChart(forecast, observed) {
  const hours = new Map();
  for (const point of forecast?.waitTimes || []) hours.set(point.time, { forecast: point.minutes });
  for (const point of observed?.waitTimes || []) {
    hours.set(point.time, { ...(hours.get(point.time) || {}), observed: point.minutes });
  }
  if (hours.size === 0) return "";
  const entries = [...hours.entries()].sort(([a], [b]) => new Date(a) - new Date(b));
  const max = Math.max(5, ...entries.flatMap(([, value]) => [value.forecast || 0, value.observed || 0]));

  const columns = entries.map(([, value]) => {
    const bar = value.forecast !== undefined
      ? `<span class="bar-value">${Math.round(value.forecast)}</span><div class="bar" style="height:${(value.forecast / max) * 85}%"></div>`
      : "";
    const line = value.observed !== undefined
      ? `<div class="bar-observed" style="bottom:${(value.observed / max) * 85}%" title="Observé : ${minutes(value.observed)}"></div>`
      : "";
    return `<div class="bar-col">${bar}${line}</div>`;
  }).join("");
  const labels = entries.map(([time]) => `<span>${formatHour(time)}</span>`).join("");
  const legend = observed ? `<p class="muted">Barres : prévision · trait : temps d'attente observé</p>` : "";
  return `<h3>Attente moyenne par heure (minutes)</h3><div class="bars">${columns}</div><div class="bar-labels">${labels}</div>${legend}`;
}

function historyTable(history) {
  if (!history || history.length < 2) return "";
  const rows = history.slice(0, 10).map((item) => `<tr>
      <td>${dateTimeFormat.format(new Date(item.forecastedAt))}</td>
      <td>${minutes(item.avgWait)}</td>
      <td>${LEVELS[levelOf(item.crowdScore) - 1].label}</td>
      <td>${Math.round(item.confidence * 100)} %</td>
    </tr>`).join("");
  return `<h3>Historique des prévisions</h3><div class="table-scroll"><table>
    <thead><tr><th>Prévu le</th><th>Attente</th><th>Affluence</th><th>Confiance</th></tr></thead>
    <tbody>${rows}</tbody></table></div>`;
}

function slotSection(slot) {
  const forecast = slot.latestForecast;
  const observed = slot.observed;
  const title = slot.dayType === "EVENT" ? "🎃 Soirée événement" : "Journée";
  let html = `<h3>${title} · ${formatTime(slot.startDate)} – ${formatTime(slot.endDate)}</h3>`;

  const stats = [];
  if (forecast) {
    const level = levelOf(forecast.crowdScore);
    stats.push(`<div class="stat"><div class="value"><span class="swatch level-${level}"></span> ${LEVELS[level - 1].label}</div><div class="label">Affluence prévue</div></div>`);
    stats.push(`<div class="stat"><div class="value">${minutes(forecast.avgWait)}</div><div class="label">Attente moyenne prévue</div></div>`);
    stats.push(`<div class="stat"><div class="value">${Math.round(forecast.confidence * 100)} %</div><div class="label">Confiance</div></div>`);
  }
  if (observed) {
    stats.push(`<div class="stat"><div class="value">${minutes(observed.avgWait)}</div><div class="label">Attente moyenne observée</div></div>`);
  }
  if (stats.length) html += `<div class="stats">${stats.join("")}</div>`;
  else html += `<p class="muted">La prévision de ce créneau est en cours de calcul.</p>`;

  html += featureChips(observed?.features || forecast?.features);
  html += hourlyChart(forecast, observed);
  html += historyTable(slot.history);
  if (forecast) {
    html += `<p class="muted">Dernière prévision le ${dateTimeFormat.format(new Date(forecast.forecastedAt))}${forecast.modelSamples ? `, modèle entraîné sur ${forecast.modelSamples} créneaux observés` : ", modèle a priori (pas encore assez d'observations)"}.</p>`;
  }
  return html;
}

async function showDay(date) {
  selectedDay = date;
  document.querySelectorAll(".day.selected").forEach((element) => element.classList.remove("selected"));
  document.querySelector(`.day[data-date="${date}"]`)?.classList.add("selected");

  const section = document.getElementById("detail");
  const content = document.getElementById("detail-content");
  section.hidden = false;
  content.innerHTML = `<p class="muted">Chargement…</p>`;

  try {
    const { slots } = await getJSON(`/api/days/${date}`);
    const active = slots.filter((slot) => slot.active);
    content.innerHTML = `<button class="close" type="button" aria-label="Fermer">✕</button>
      <h2 style="text-transform: capitalize">${longDateFormat.format(parseDay(date))}</h2>
      ${(active.length ? active : slots).map(slotSection).join("")}`;
  } catch (error) {
    content.innerHTML = `<button class="close" type="button" aria-label="Fermer">✕</button><p class="muted">${escapeHTML(error.message)}</p>`;
  }
  content.querySelector(".close").addEventListener("click", () => {
    section.hidden = true;
    selectedDay = null;
    document.querySelectorAll(".day.selected").forEach((element) => element.classList.remove("selected"));
  });
  section.scrollIntoView({ behavior: "smooth", block: "nearest" });
}

async function renderLive() {
  const container = document.getElementById("live-content");
  let snapshot;
  try {
    snapshot = await getJSON("/api/live");
  } catch (error) {
    container.textContent = error.status === 404 ? "Aucun temps d'attente collecté pour l'instant." : `Indisponible : ${error.message}`;
    return;
  }

  const age = (Date.now() - new Date(snapshot.observedAt)) / 60000;
  if (age > 30) {
    container.innerHTML = `Le parc est fermé. Derniers temps d'attente relevés le ${dateTimeFormat.format(new Date(snapshot.observedAt))}.`;
    return;
  }

  const rides = (snapshot.rides || [])
    .filter((ride) => ride.status === "OPERATING" && typeof ride.waitTime === "number")
    .sort((a, b) => b.waitTime - a.waitTime);
  const level = snapshot.avgWait != null ? levelOf(Math.log(snapshot.avgWait / 5) / Math.log(12)) : null;

  container.classList.remove("muted");
  container.innerHTML = `<div class="stats">
      <div class="stat"><div class="value">${snapshot.avgWait != null ? minutes(snapshot.avgWait) : "–"}</div><div class="label">Attente moyenne</div></div>
      ${level ? `<div class="stat"><div class="value"><span class="swatch level-${level}"></span> ${LEVELS[level - 1].label}</div><div class="label">Affluence actuelle</div></div>` : ""}
      <div class="stat"><div class="value">${snapshot.operating}</div><div class="label">Attractions ouvertes</div></div>
    </div>
    <ul class="rides">${rides.map((ride) => `<li><span>${escapeHTML(ride.name)}</span><strong>${minutes(ride.waitTime)}</strong></li>`).join("")}</ul>
    <p class="muted">Relevé à ${formatTime(snapshot.observedAt)}.</p>`;
}

async function renderModel() {
  const container = document.getElementById("model-content");
  try {
    const { model, accuracy, observedSlots } = await getJSON("/api/model");
    const parts = [];
    if (model.samples > 0) {
      parts.push(`Le modèle est entraîné sur <strong>${model.samples}</strong> créneaux observés (écart type ±${Math.round((Math.exp(model.sigma) - 1) * 100)} %).`);
    } else {
      parts.push(`Le modèle utilise encore ses hypothèses de départ : il s'affinera automatiquement à mesure que les temps d'attente sont collectés (${observedSlots} créneau${observedSlots > 1 ? "x" : ""} observé${observedSlots > 1 ? "s" : ""} pour l'instant).`);
    }
    if (accuracy.slots > 0) {
      parts.push(`Sur ${accuracy.slots} créneaux passés, l'attente moyenne prévue la veille s'est écartée de <strong>${accuracy.mae} min</strong> en moyenne (biais ${accuracy.bias > 0 ? "+" : ""}${accuracy.bias} min).`);
    }
    parts.push(`Facteurs pris en compte : jour de la semaine, vacances scolaires par zone, jours fériés et ponts, soirées événements, amplitude d'ouverture et météo.`);
    container.classList.remove("muted");
    container.innerHTML = parts.map((part) => `<p>${part}</p>`).join("");
  } catch (error) {
    container.textContent = `Indisponible : ${error.message}`;
  }
}

document.getElementById("calendar").addEventListener("click", (event) => {
  const button = event.target.closest("button.day");
  if (button) showDay(button.dataset.date);
});

renderLegend();
renderCalendar();
renderLive();
renderModel();
setInterval(renderLive, 5 * 60 * 1000);
