import { useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../app/auth-context";

// Switch sessions on a public route so the protected-route redirect cannot race signup.
export default function NewWorkspace() {
  const { isAuthed, logout } = useAuth();
  const navigate = useNavigate();
  const switched = useRef(false);
  useEffect(() => {
    if (switched.current) return;
    switched.current = true;
    if (isAuthed) logout();
    navigate("/register?next=restore", { replace: true });
  }, [isAuthed, logout, navigate]);
  return (
    <p role="status" className="spending-empty">
      Preparing a new workspace…
    </p>
  );
}
