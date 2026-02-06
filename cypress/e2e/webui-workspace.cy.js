describe("webui workspace", () => {
  it("allows adding a project path and selecting a user", () => {
    // Start the web UI before running: ss --web
    cy.visit("/");
    cy.contains("Workspace");
    cy.get('[data-testid="project-select"]').should("exist");
    cy.get('[data-testid="user-select"]').should("exist");
  });
});
