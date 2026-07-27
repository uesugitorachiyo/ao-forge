package cli

type directWorkspaceSession struct {
	path string
}

func newDirectWorkspaceSession(plan factoryPlan, workcell *workcellRunState) directWorkspaceSession {
	path := plan.Objective.Workspace
	if workcell.Workspace != "" {
		path = workcell.Workspace
	}
	return directWorkspaceSession{path: path}
}

func (session directWorkspaceSession) Path() string {
	return session.path
}

func (directWorkspaceSession) Close() error {
	return nil
}
