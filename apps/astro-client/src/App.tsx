import { BrowserRouter, Routes, Route } from "react-router-dom";
import { Layout } from "./components/Layout";
import { Home } from "./pages/Home";
import { Hire } from "./pages/Hire";
import { AgentDetail } from "./pages/AgentDetail";
import { RequestAgent } from "./pages/RequestAgent";
import { YourAgents } from "./pages/YourAgents";
import { NotFound } from "./pages/NotFound";

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Home />} />
          <Route path="hire" element={<Hire />} />
          <Route path="hire/:agentSlug" element={<AgentDetail />} />
          <Route path="request-agent" element={<RequestAgent />} />
          <Route path="agents" element={<YourAgents />} />
          <Route path="*" element={<NotFound />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;
