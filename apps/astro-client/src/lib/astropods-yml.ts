export function generateAstropodsYml(name: string): string {
  return [
    `spec: package/v1`,
    `name: ${name}`,
    ``,
    `agent:`,
    `  build:`,
    `    context: .`,
    `    dockerfile: Dockerfile`,
  ].join("\n");
}
