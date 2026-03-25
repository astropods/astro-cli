import {
  configure,
  getConsoleSink,
  getLogger,
  ansiColorFormatter,
  type LogLevel,
} from "@logtape/logtape";

const LOG_LEVEL = (process.env.LOG_LEVEL || "info") as LogLevel;
const LOG_FORMAT = (process.env.LOG_FORMAT || "text").toLowerCase();

function jsonFormatter(record: Parameters<typeof ansiColorFormatter>[0]) {
  const entry = {
    level: record.level,
    category: record.category.join("."),
    msg: record.message.join(""),
    time: record.timestamp,
    ...record.properties,
  };
  return [JSON.stringify(entry)];
}

await configure({
  reset: true,
  sinks: {
    console: getConsoleSink({
      formatter: LOG_FORMAT === "json" ? jsonFormatter : ansiColorFormatter,
    }),
  },
  loggers: [
    { category: ["astro-client"], lowestLevel: LOG_LEVEL, sinks: ["console"] },
    { category: ["logtape", "meta"], lowestLevel: "warning", sinks: ["console"] },
  ],
});

const log = getLogger(["astro-client"]);

export default log;

export function getChildLogger(name: string) {
  return getLogger(["astro-client", name]);
}
