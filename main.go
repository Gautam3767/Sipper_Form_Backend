package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Order represents the order form payload.
type Order struct {
	ID                  primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ProductType         string             `bson:"productType" json:"productType"`
	SubOption           string             `bson:"subOption" json:"subOption"`
	OrderType           string             `bson:"orderType" json:"orderType"`
	BrandName           string             `bson:"brandName,omitempty" json:"brandName"`
	Quantity            string             `bson:"quantity" json:"quantity"`
	Size                string             `bson:"size" json:"size"`
	DeliveryDate        string             `bson:"deliveryDate" json:"deliveryDate"`
	DeliveryTime        string             `bson:"deliveryTime" json:"deliveryTime"`
	SpecialInstructions string             `bson:"specialInstructions,omitempty" json:"specialInstructions,omitempty"`
	TermsAccepted       bool               `bson:"termsAccepted" json:"termsAccepted"`
	CompanyName         string             `bson:"companyName" json:"companyName"`
	Email               string             `bson:"email" json:"email"`
	PhoneNumber         string             `bson:"phoneNumber" json:"phoneNumber"`
	Address             string             `bson:"address" json:"address"`
	CreatedAt           time.Time          `bson:"createdAt" json:"createdAt"`
	// DeliveryDateTime is the parsed combination of delivery date and time.
	DeliveryDateTime time.Time `bson:"deliveryDateTime" json:"deliveryDateTime"`
}

var (
	client    *mongo.Client
	orderColl *mongo.Collection
)

func main() {
	// Load environment variables from .env file.
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: Could not load .env file")
	}

	// Retrieve MongoDB URI and PORT from environment variables.
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Fatal("MONGODB_URI not set in environment")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err = mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		log.Fatalf("Error connecting to MongoDB: %v", err)
	}

	// Verify the connection.
	if err = client.Ping(context.Background(), nil); err != nil {
		log.Fatalf("Error pinging MongoDB: %v", err)
	}

	// Use a specific database and collection.
	orderColl = client.Database("orderdb").Collection("orders")

	// Setup HTTP endpoint with CORS middleware.
	handler := enableCors(http.HandlerFunc(orderHandler))
	http.Handle("/order", handler)

	log.Printf("Server starting on port %s...", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// enableCors adds CORS headers to the response.
func enableCors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// orderHandler processes incoming POST requests with order data.
func orderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST is allowed", http.StatusMethodNotAllowed)
		return
	}

	var order Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		http.Error(w, "Invalid JSON data", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate the order payload.
	if err := validateOrder(order); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Convert quantity from string to integer.
	_, err := strconv.Atoi(order.Quantity)
	if err != nil {
		http.Error(w, "Quantity must be a valid number", http.StatusBadRequest)
		return
	}

	// Parse and combine delivery date and time.
	deliveryDateTime, err := parseDeliveryDateTime(order.DeliveryDate, order.DeliveryTime)
	if err != nil {
		http.Error(w, "Invalid delivery date or time format", http.StatusBadRequest)
		return
	}

	// Set server-managed fields.
	order.CreatedAt = time.Now()
	order.DeliveryDateTime = deliveryDateTime

	// Insert the order into MongoDB.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := orderColl.InsertOne(ctx, order)
	if err != nil {
		log.Printf("Error inserting order: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Respond with order ID and confirmation.
	response := map[string]interface{}{
		"orderID": res.InsertedID,
		"status":  "Order received",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// validateOrder checks required fields and validates business logic.
func validateOrder(o Order) error {
	if o.ProductType == "" ||
		o.SubOption == "" ||
		o.Quantity == "" ||
		o.Size == "" ||
		o.DeliveryDate == "" ||
		o.DeliveryTime == "" ||
		o.CompanyName == "" ||
		o.Email == "" ||
		o.PhoneNumber == "" ||
		o.Address == "" {
		return errors.New("missing required fields")
	}

	// For "Existing Brand" orders, ensure brandName is provided and quantity meets minimum requirements.
	if o.OrderType == "Existing Brand" {
		if o.BrandName == "" {
			return errors.New("brandName is required for Existing Brand orders")
		}
		quantity, err := strconv.Atoi(o.Quantity)
		if err != nil {
			return errors.New("quantity must be a valid number")
		}
		if quantity < 1000 {
			return errors.New("quantity must be at least 1000 for Existing Brand orders")
		}
	}

	// Basic email validation.
	if !isValidEmail(o.Email) {
		return errors.New("invalid email format")
	}

	return nil
}

// parseDeliveryDateTime combines deliveryDate and deliveryTime into a single time.Time value.
// Assumes date format "YYYY-MM-DD" and time format "HH:MM".
func parseDeliveryDateTime(dateStr, timeStr string) (time.Time, error) {
	layout := "2006-01-02 15:04"
	combined := fmt.Sprintf("%s %s", dateStr, timeStr)
	return time.Parse(layout, combined)
}

// isValidEmail provides a basic check for the presence of "@".
func isValidEmail(email string) bool {
	if len(email) < 3 || len(email) > 254 {
		return false
	}
	for _, c := range email {
		if c == '@' {
			return true
		}
	}
	return false
}
