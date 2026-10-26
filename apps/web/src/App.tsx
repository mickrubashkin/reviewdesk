import { Loader, Stack, Text } from "@mantine/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate, Route, Routes, useNavigate, useParams } from "react-router-dom";

import { AIConfig } from "./features/ai-config/AIConfig";
import { BitrixVariables } from "./features/bitrix-variables/BitrixVariables";
import { MyTasks } from "./features/my-tasks/MyTasks";
import { AIAnalysisLogs } from "./features/ai-logs/AIAnalysisLogs";
import { AdminUsers } from "./features/admin-users/AdminUsers";
import { AuthEvents } from "./features/auth-events/AuthEvents";
import { BoardHomeRedirect, EmailBoardRoute } from "./features/boards/BoardHome";
import { Login } from "./features/auth/Login";
import { EmailEvents } from "./features/email-events/EmailEvents";
import { OperationalEvents } from "./features/ops-events/OperationalEvents";
import { demoMode, fetchCurrentUserOrEnterDemo, logout } from "./features/auth/api";
import { DemoGuide } from "./features/demo-guide/DemoGuide";
import { EmailCreate } from "./features/emails/EmailCreate";
import type { AuthUser } from "./features/emails/types";
import { EmailReview } from "./features/review/EmailReview";
import { SlotCreate } from "./features/slots/SlotCreate";
import { PortalSync } from "./features/portal/PortalSync";
import { SharedTexts } from "./features/shared-texts/SharedTexts";
import { TranslationGlossary } from "./features/translation-glossary/TranslationGlossary";
import { Blocks } from "./features/blocks/Blocks";

export default function App() {
  return <AuthenticatedApp />;
}

function AuthenticatedApp() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const currentUserQuery = useQuery({
    queryKey: ["auth", "me"],
    queryFn: fetchCurrentUserOrEnterDemo,
    retry: false,
  });
  const logoutMutation = useMutation({
    mutationFn: logout,
    onSettled: () => {
      queryClient.removeQueries();
      navigate("/");
    },
  });

  if (currentUserQuery.isLoading) {
    return (
      <Stack align="center" justify="center" h="100dvh">
        <Loader />
        <Text c="dimmed">{demoMode ? "Opening the demo" : "Checking session"}</Text>
      </Stack>
    );
  }

  if (currentUserQuery.isError || !currentUserQuery.data) {
    return <Login />;
  }

  return (
    <>
    {demoMode ? <DemoGuide currentUser={currentUserQuery.data} /> : null}
    <Routes>
      <Route path="/ai-logs" element={<AIAnalysisLogs />} />
      <Route path="/my-tasks" element={<MyTasks currentUser={currentUserQuery.data} />} />
      <Route
        path="/admin/ai-config"
        element={<AIConfig currentUserRole={currentUserQuery.data.role} />}
      />
      <Route
        path="/admin/variables"
        element={<BitrixVariables currentUserRole={currentUserQuery.data.role} />}
      />
      <Route
        path="/admin/glossary"
        element={<TranslationGlossary currentUserRole={currentUserQuery.data.role} />}
      />
      <Route
        path="/admin/blocks"
        element={<Blocks currentUserRole={currentUserQuery.data.role} />}
      />
      <Route
        path="/admin/shared-texts"
        element={<SharedTexts currentUser={currentUserQuery.data} />}
      />
      <Route
        path="/admin/portal"
        element={<PortalSync currentUserRole={currentUserQuery.data.role} />}
      />
      <Route path="/auth-events" element={<AuthEvents />} />
      <Route
        path="/admin/users"
        element={<AdminUsers currentUserRole={currentUserQuery.data.role} />}
      />
      <Route path="/admin/email-events" element={<EmailEvents />} />
      <Route path="/admin/operational-events" element={<OperationalEvents />} />
      <Route
        path="/slots/new"
        element={<SlotCreate currentUser={currentUserQuery.data} />}
      />
      <Route
        path="/emails/new"
        element={<EmailCreate currentUser={currentUserQuery.data} />}
      />
      <Route
        path="/emails/:emailId/edit"
        element={<EmailEditRedirectRoute />}
      />
      <Route
        path="/emails/:emailId/review"
        element={
          <EmailReviewRoute currentUser={currentUserQuery.data} />
        }
      />
      <Route
        path="/"
        element={<BoardHomeRedirect />}
      />
      <Route
        path="/boards/:boardKey"
        element={
          <EmailBoardRoute
            currentUser={currentUserQuery.data}
            isLoggingOut={logoutMutation.isPending}
            onLogout={() => logoutMutation.mutate()}
          />
        }
      />
      <Route path="*" element={<Navigate replace to="/" />} />
    </Routes>
    </>
  );
}

function EmailReviewRoute({ currentUser }: { currentUser: AuthUser }) {
  const { emailId } = useParams();
  return emailId ? (
    <EmailReview currentUser={currentUser} emailId={emailId} />
  ) : (
    <Navigate replace to="/" />
  );
}

function EmailEditRedirectRoute() {
  const { emailId } = useParams();
  return emailId ? (
    <Navigate replace to={`/emails/${encodeURIComponent(emailId)}/review`} />
  ) : (
    <Navigate replace to="/" />
  );
}
