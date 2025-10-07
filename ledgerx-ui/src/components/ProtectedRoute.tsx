import { Navigate } from "react-router-dom";
import { useAuth } from "../app/auth";

type Props = { children: React.ReactElement | React.ReactElement[] };

export default function ProtectedRoute({ children }: Props) {
  const { token } = useAuth();
  return token ? <>{children}</> : <Navigate to="/login" replace />;
}
