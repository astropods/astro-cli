export function generateAstropodsYml(name: string, visibility: string): string {
  return [
    `spec: package/v1`,
    `name: ${name}`,
    ``,
    `meta:`,
    `  visibility: ${visibility}`,
    ``,
    `agent:`,
    `  build:`,
    `    context: .`,
    `    dockerfile: Dockerfile`,
  ].join("\n");
}
