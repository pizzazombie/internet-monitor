const palette = {
  up: "#20805f",
  degraded: "#c78b1e",
  down: "#b43d2e",
  grid: "rgba(36, 25, 11, 0.12)",
  text: "#24190b",
};

const state = {
  hours: 24,
  timer: null,
};

function toLocalInputValue(date) {
  const offset = date.getTimezoneOffset();
  const local = new Date(date.getTime() - offset * 60_000);
  return local.toISOString().slice(0, 16);
}

function fromLocalInputValue(value) {
  if (!value) return null;
  return new Date(value);
}

function formatDuration(seconds) {
  if (!seconds) return "0m";
  const parts = [];
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days) parts.push(`${days}d`);
  if (hours) parts.push(`${hours}h`);
  if (minutes || parts.length === 0) parts.push(`${minutes}m`);
  return parts.join(" ");
}

function formatTimestamp(value) {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function setupRangeInputs() {
  const now = new Date();
  document.getElementById("to").value = toLocalInputValue(now);
  document.getElementById("from").value = toLocalInputValue(
    new Date(now.getTime() - state.hours * 3600_000),
  );
}

function setActiveButton(hours) {
  document.querySelectorAll("[data-hours]").forEach((button) => {
    button.classList.toggle("active", Number(button.dataset.hours) === hours);
  });
}

async function loadOverview() {
  const from = fromLocalInputValue(document.getElementById("from").value);
  const to = fromLocalInputValue(document.getElementById("to").value);
  if (!from || !to || to <= from) return;

  const query = new URLSearchParams({
    from: from.toISOString(),
    to: to.toISOString(),
  });
  const response = await fetch(`/api/overview?${query.toString()}`);
  const payload = await response.json();
  render(payload);
}

function render(payload) {
  const { summary, timeline, latency, speed } = payload;
  document.getElementById("uptime").textContent = `${summary.uptime_pct.toFixed(2)}%`;
  document.getElementById("outages").textContent = `${summary.outages}`;
  document.getElementById("downtime").textContent = formatDuration(summary.downtime_sec);
  document.getElementById("latency").textContent = summary.avg_latency_ms
    ? `${summary.avg_latency_ms.toFixed(0)} ms`
    : "n/a";
  document.getElementById("last-status").textContent = summary.last_status || "no data";

  const strip = document.getElementById("status-strip");
  strip.className = `status-strip ${summary.last_status || ""}`;
  if (!summary.last_status) {
    strip.textContent = "No checks yet. The monitor needs a couple of intervals to collect data.";
  } else {
    strip.textContent = `Last check ${formatTimestamp(summary.last_checked_at)}: ${summary.last_status}.`;
  }

  renderLegend(timeline);
  drawTimeline(document.getElementById("timeline-chart"), timeline, summary.from, summary.to);
  drawLineChart(document.getElementById("latency-chart"), latency, "ms");
  drawLineChart(document.getElementById("speed-chart"), speed, "Mbps");
}

function renderLegend(timeline) {
  const counts = { up: 0, degraded: 0, down: 0 };
  timeline.forEach((item) => {
    counts[item.status] += 1;
  });

  const legend = document.getElementById("timeline-legend");
  legend.innerHTML = ["up", "degraded", "down"]
    .map(
      (key) =>
        `<span><i style="background:${palette[key]}"></i>${key}: ${counts[key] ?? 0} segments</span>`,
    )
    .join("");
}

function scaleCanvas(canvas) {
  const ratio = window.devicePixelRatio || 1;
  const width = canvas.clientWidth;
  const height = canvas.clientHeight;
  canvas.width = Math.floor(width * ratio);
  canvas.height = Math.floor(height * ratio);
  const ctx = canvas.getContext("2d");
  ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
  return { ctx, width, height };
}

function drawTimeline(canvas, timeline, from, to) {
  const { ctx, width, height } = scaleCanvas(canvas);
  ctx.clearRect(0, 0, width, height);
  ctx.fillStyle = "rgba(36, 25, 11, 0.05)";
  ctx.fillRect(0, 0, width, height);

  if (!timeline.length) {
    drawEmpty(ctx, width, height, "No timeline data");
    return;
  }

  const start = new Date(from).getTime();
  const end = new Date(to).getTime();
  const total = Math.max(end - start, 1);
  const top = 18;
  const chartHeight = height - top * 2;

  timeline.forEach((item) => {
    const x1 = ((new Date(item.start).getTime() - start) / total) * width;
    const x2 = ((new Date(item.end).getTime() - start) / total) * width;
    ctx.fillStyle = palette[item.status] || palette.down;
    ctx.fillRect(x1, top, Math.max(x2 - x1, 1.5), chartHeight);
  });
}

function drawLineChart(canvas, samples, unit) {
  const { ctx, width, height } = scaleCanvas(canvas);
  ctx.clearRect(0, 0, width, height);
  ctx.fillStyle = "rgba(36, 25, 11, 0.03)";
  ctx.fillRect(0, 0, width, height);

  if (!samples.length) {
    drawEmpty(ctx, width, height, `No ${unit} samples`);
    return;
  }

  const padding = { top: 18, right: 18, bottom: 26, left: 44 };
  const chartWidth = width - padding.left - padding.right;
  const chartHeight = height - padding.top - padding.bottom;

  const xMin = new Date(samples[0].ts).getTime();
  const xMax = new Date(samples[samples.length - 1].ts).getTime();
  const yMax = Math.max(...samples.map((sample) => sample.value), 1);

  ctx.strokeStyle = palette.grid;
  ctx.lineWidth = 1;
  for (let i = 0; i < 4; i += 1) {
    const y = padding.top + (chartHeight / 3) * i;
    ctx.beginPath();
    ctx.moveTo(padding.left, y);
    ctx.lineTo(width - padding.right, y);
    ctx.stroke();
  }

  ctx.strokeStyle = unit === "Mbps" ? palette.degraded : palette.up;
  ctx.lineWidth = 2;
  ctx.beginPath();
  samples.forEach((sample, index) => {
    const x =
      padding.left +
      ((new Date(sample.ts).getTime() - xMin) / Math.max(xMax - xMin, 1)) * chartWidth;
    const y = padding.top + chartHeight - (sample.value / yMax) * chartHeight;
    if (index === 0) ctx.moveTo(x, y);
    else ctx.lineTo(x, y);
  });
  ctx.stroke();

  ctx.fillStyle = palette.text;
  ctx.font = "12px Avenir Next, sans-serif";
  ctx.fillText(`0 ${unit}`, 8, height - 8);
  ctx.fillText(`${yMax.toFixed(1)} ${unit}`, 8, padding.top + 8);
}

function drawEmpty(ctx, width, height, label) {
  ctx.fillStyle = "rgba(36, 25, 11, 0.55)";
  ctx.font = "14px Avenir Next, sans-serif";
  ctx.textAlign = "center";
  ctx.fillText(label, width / 2, height / 2);
}

function wireControls() {
  document.querySelectorAll("[data-hours]").forEach((button) => {
    button.addEventListener("click", async () => {
      state.hours = Number(button.dataset.hours);
      const now = new Date();
      document.getElementById("to").value = toLocalInputValue(now);
      document.getElementById("from").value = toLocalInputValue(
        new Date(now.getTime() - state.hours * 3600_000),
      );
      setActiveButton(state.hours);
      await loadOverview();
    });
  });

  document.getElementById("apply-range").addEventListener("click", async () => {
    setActiveButton(-1);
    await loadOverview();
  });

  window.addEventListener("resize", () => {
    loadOverview().catch(console.error);
  });
}

async function boot() {
  setupRangeInputs();
  setActiveButton(state.hours);
  wireControls();
  await loadOverview();
  state.timer = window.setInterval(() => {
    loadOverview().catch(console.error);
  }, 30_000);
}

boot().catch((error) => {
  const strip = document.getElementById("status-strip");
  strip.className = "status-strip down";
  strip.textContent = `Failed to load monitor data: ${error.message}`;
});
