describe("webui workspace", () => {
  it("allows adding a project path and selecting a user", () => {
    // Start the web UI before running: ss --web
    cy.visit("/");
    cy.contains("Workspace");
    cy.get('[data-testid="project-select"]').should("exist");
    cy.get('[data-testid="user-select"]').should("exist");
  });

  it("opens task editor modal when creating a new task", () => {
    cy.visit("/");
    cy.get('[data-testid="new-task"]').click();
    cy.get('[data-testid="task-modal"]').should("be.visible");
    cy.get('[data-testid="task-intent"]').type("New task");
    cy.get('[data-testid="task-hours"]').clear().type("2");
    cy.get('[data-testid="task-save"]').click();
    cy.get('[data-testid="task-modal"]').should("not.be.visible");
    cy.contains("New task");
  });

  it("toggles the actions dropdown menu", () => {
    cy.visit("/");
    cy.get('[data-testid="action-button"]').click();
    cy.get('[data-testid="action-list"]').should("be.visible");
    cy.get("body").click(5, 5);
    cy.get('[data-testid="action-list"]').should("not.be.visible");
  });
});
