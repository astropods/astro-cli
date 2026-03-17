import homepage from "./dev.html";

Bun.serve({
  port: 3333,
  routes: {
    "/": homepage,
    "/assets/openclaw.png": new Response(Bun.file(import.meta.dir + "/assets/openclaw.png")),
  },
  development: true,
});

console.log("Trading card dev server → http://localhost:3333");
