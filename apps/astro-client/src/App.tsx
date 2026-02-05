import { BrowserRouter, Routes, Route } from "react-router-dom";
import { AuthProvider } from "./lib/auth";
import { Layout } from "./components/Layout";
import { Home } from "./pages/Home";
import { Hire } from "./pages/Hire";
import { AgentDetail } from "./pages/AgentDetail";
import { RequestAgent } from "./pages/RequestAgent";
import { YourAgents } from "./pages/YourAgents";
import { Operator } from "./pages/Operator";
import { NotFound } from "./pages/NotFound";

function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<Layout />}>
            <Route index element={<Home />} />
            <Route path="hire" element={<Hire />} />
            <Route path="hire/:agentSlug" element={<AgentDetail />} />
            <Route path="request-agent" element={<RequestAgent />} />
            <Route path="agents" element={<YourAgents />} />
            <Route path="operator" element={<Operator />} />
            <Route path="*" element={<NotFound />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}

export default App;
