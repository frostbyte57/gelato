import { Navigate, Route, Routes } from "react-router-dom";
import { DashboardPage } from "./routes/DashboardPage";
import { ConnectionsPage } from "./routes/ConnectionsPage";

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<DashboardPage />} />
      <Route path="/connections" element={<ConnectionsPage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

