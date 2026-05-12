interface SmokeEnvConfig {
  loginUrlPattern: RegExp;
  loginUrlExclude: (url: string) => boolean;
  apiDomain: string;
  appBaseUrl: string;
  cliName: string;
  deviceUrlPattern: RegExp;
  minExploreCards: number;
  minWeatherPoetDeploys: number;
  authorDisplayName: RegExp;
  username: string;
}

const prod: SmokeEnvConfig = {
  loginUrlPattern: /login\.astropods\.com/,
  loginUrlExclude: (url) => !url.includes("login.astropods.com"),
  apiDomain: "astropods.com",
  appBaseUrl: "https://astropods.com",
  cliName: "ast",
  deviceUrlPattern: /https:\/\/login\.astropods\.com\/device\?user_code=[A-Z0-9-]+/,
  minExploreCards: 7,
  minWeatherPoetDeploys: 1,
  authorDisplayName: /Rodric Rabbah/,
  username: "astro-testbot",
};

const preview: SmokeEnvConfig = {
  loginUrlPattern: /authkit\.app/,
  loginUrlExclude: (url) => !url.includes("authkit.app"),
  apiDomain: "astropod.ai",
  appBaseUrl: "https://astropod.ai",
  cliName: "ast-preview",
  deviceUrlPattern: /https:\/\/[^\s]+\/device\?user_code=[A-Z0-9-]+/,
  minExploreCards: 5,
  minWeatherPoetDeploys: 1,
  authorDisplayName: /r r/,
  username: "astro-testbot",
};

const dev: SmokeEnvConfig = {
  ...preview,
  apiDomain: "localhost",
  appBaseUrl: "http://localhost",
  cliName: "ast-dev",
  minExploreCards: 0,
  minWeatherPoetDeploys: 0,
  authorDisplayName: /.*/,
  username: process.env.ASTRO_TEST_USERNAME ?? "",
};

const ASTRO_ENV = process.env.ASTRO_ENV;
if (ASTRO_ENV !== "dev" && ASTRO_ENV !== "preview" && ASTRO_ENV !== "prod") {
  throw new Error(
    `ASTRO_ENV must be "dev", "preview", or "prod" (got ${JSON.stringify(ASTRO_ENV)}). ` +
    `Run via "moon run tests:smoke" or set ASTRO_ENV explicitly.`
  );
}

export const envConfig: SmokeEnvConfig =
  ASTRO_ENV === "prod" ? prod : ASTRO_ENV === "preview" ? preview : dev;
