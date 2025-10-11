package main

import (
	"fmt"
	"log"

	"authway/src/server/pkg/tenant"
	"authway/src/server/pkg/user"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Connect to PostgreSQL
	connStr := "host=localhost port=5432 user=authway password=authway dbname=authway sslmode=disable"
	db, err := gorm.Open(postgres.Open(connStr), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	fmt.Println("🧪 Running Integration Tests...")
	fmt.Println()

	// Cleanup any leftover test data from previous runs
	fmt.Println("🧹 Pre-test cleanup...")
	db.Exec("DELETE FROM users WHERE email LIKE '%integration%'")
	db.Exec("DELETE FROM tenants WHERE slug LIKE '%int%'")
	fmt.Println("  ✓ Cleaned up previous test data")
	fmt.Println()

	// Test 1: Tenant Service
	fmt.Println("📋 Test 1: Tenant Service")
	tenantService := tenant.NewService(db)

	// Create a test tenant
	testTenant, err := tenantService.CreateTenant(tenant.CreateTenantRequest{
		Name:        "Test Tenant Integration",
		Slug:        "test-tenant-int",
		Description: "Integration test tenant",
	})
	if err != nil {
		log.Fatal("Failed to create tenant:", err)
	}
	fmt.Printf("  ✓ Created tenant: %s (ID: %s)\n", testTenant.Name, testTenant.ID)

	// Get tenant by ID
	fetchedTenant, err := tenantService.GetTenantByID(testTenant.ID)
	if err != nil {
		log.Fatal("Failed to get tenant by ID:", err)
	}
	fmt.Printf("  ✓ Fetched tenant by ID: %s\n", fetchedTenant.Name)

	// Get tenant by slug
	fetchedBySlug, err := tenantService.GetTenantBySlug(testTenant.Slug)
	if err != nil {
		log.Fatal("Failed to get tenant by slug:", err)
	}
	fmt.Printf("  ✓ Fetched tenant by slug: %s\n", fetchedBySlug.Name)

	// Test 2: User Service - Tenant Isolation
	fmt.Println()
	fmt.Println("📋 Test 2: User Service - Tenant Isolation")
	userService := user.NewService(db, logger)

	// Create another tenant for isolation test
	tenant2, err := tenantService.CreateTenant(tenant.CreateTenantRequest{
		Name: "Tenant 2 Integration",
		Slug: "tenant-2-int",
	})
	if err != nil {
		log.Fatal("Failed to create tenant 2:", err)
	}
	fmt.Printf("  ✓ Created tenant 2: %s\n", tenant2.Name)

	// Create user with same email in both tenants
	email := "test@integration.com"

	user1, err := userService.Create(testTenant.ID, &user.CreateUserRequest{
		Email:    email,
		Password: "password123",
		Name:     "User in Tenant 1",
	})
	if err != nil {
		log.Fatal("Failed to create user 1:", err)
	}
	fmt.Printf("  ✓ Created user in tenant 1: %s\n", user1.Email)

	user2, err := userService.Create(tenant2.ID, &user.CreateUserRequest{
		Email:    email,
		Password: "password456",
		Name:     "User in Tenant 2",
	})
	if err != nil {
		log.Fatal("Failed to create user 2:", err)
	}
	fmt.Printf("  ✓ Created user in tenant 2 with same email: %s\n", user2.Email)

	// Verify isolation
	if user1.TenantID != testTenant.ID {
		log.Fatal("User 1 tenant mismatch")
	}
	if user2.TenantID != tenant2.ID {
		log.Fatal("User 2 tenant mismatch")
	}
	if user1.ID == user2.ID {
		log.Fatal("Users should have different IDs")
	}
	fmt.Printf("  ✓ Verified tenant isolation: different users with same email\n")

	// Test GetByEmailAndTenant
	foundUser1, err := userService.GetByEmailAndTenant(testTenant.ID, email)
	if err != nil {
		log.Fatal("Failed to get user by email and tenant:", err)
	}
	if foundUser1.ID != user1.ID {
		log.Fatal("GetByEmailAndTenant returned wrong user")
	}
	fmt.Printf("  ✓ GetByEmailAndTenant works correctly\n")

	// Test GetByTenant
	tenant1Users, total, err := userService.GetByTenant(testTenant.ID, 10, 0)
	if err != nil {
		log.Fatal("Failed to get users by tenant:", err)
	}
	fmt.Printf("  ✓ GetByTenant returned %d users (total: %d)\n", len(tenant1Users), total)

	// Test 3: Password Reset and Email Verification
	fmt.Println()
	fmt.Println("📋 Test 3: Password Operations")

	// Update password
	newPassword := "newpassword789"
	err = userService.UpdatePassword(user1.ID, newPassword)
	if err != nil {
		log.Fatal("Failed to update password:", err)
	}
	fmt.Printf("  ✓ Password updated successfully\n")

	// Verify new password
	updatedUser, err := userService.GetByID(user1.ID)
	if err != nil {
		log.Fatal("Failed to get updated user:", err)
	}
	if !userService.VerifyPassword(updatedUser, newPassword) {
		log.Fatal("New password verification failed")
	}
	fmt.Printf("  ✓ New password verified\n")

	// Update email verified
	err = userService.UpdateEmailVerified(user1.ID, true)
	if err != nil {
		log.Fatal("Failed to update email verified:", err)
	}
	verifiedUser, err := userService.GetByID(user1.ID)
	if err != nil {
		log.Fatal("Failed to get verified user:", err)
	}
	if !verifiedUser.EmailVerified {
		log.Fatal("Email verified not updated")
	}
	fmt.Printf("  ✓ Email verification status updated\n")

	// Cleanup
	fmt.Println()
	fmt.Println("🧹 Cleanup")

	// Delete users
	err = userService.Delete(user1.ID)
	if err != nil {
		log.Fatal("Failed to delete user 1:", err)
	}
	err = userService.Delete(user2.ID)
	if err != nil {
		log.Fatal("Failed to delete user 2:", err)
	}
	fmt.Printf("  ✓ Deleted test users\n")

	// Delete tenants
	err = tenantService.DeleteTenant(testTenant.ID)
	if err != nil {
		log.Fatal("Failed to delete tenant 1:", err)
	}
	err = tenantService.DeleteTenant(tenant2.ID)
	if err != nil {
		log.Fatal("Failed to delete tenant 2:", err)
	}
	fmt.Printf("  ✓ Deleted test tenants\n")

	fmt.Println()
	fmt.Println("✅ All integration tests passed!")
}
