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
};

export const envConfig: SmokeEnvConfig =
  process.env.ASTRO_ENV === "preview" ? preview : prod;
