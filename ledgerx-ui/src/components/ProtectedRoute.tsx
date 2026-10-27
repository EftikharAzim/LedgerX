import { Navigate } from "react-router-dom";
import { useAuth } from "../app/auth-context";

type Props = { children: React.ReactElement | React.ReactElement[] };

export default function ProtectedRoute({ children }: Props) {
  const { isAuthed } = useAuth();
  return isAuthed ? <>{children}</> : <Navigate to="/login" replace />;
}
