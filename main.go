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
	"strings"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
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
	DeliveryDateTime    time.Time          `bson:"deliveryDateTime" json:"deliveryDateTime"`
	Status              string             `bson:"status,omitempty" json:"status,omitempty"`
}

// StatusUpdate represents a status update request
type StatusUpdate struct {
	Status string `json:"status"`
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

	// Setup HTTP endpoints with CORS middleware.
	http.Handle("/order", enableCors(http.HandlerFunc(orderHandler)))
	http.Handle("/orders", enableCors(http.HandlerFunc(getOrdersHandler)))
	// Use a more generic handler for all paths starting with /orders/
	http.Handle("/orders/", enableCors(http.HandlerFunc(orderRouteHandler)))

	log.Printf("Server starting on port %s...", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// enableCors adds CORS headers to the response.
func enableCors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow requests from any origin
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// Allow more HTTP methods including PATCH
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, PATCH")
		// Allow additional headers
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		// Allow credentials
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		// Set max age for preflight requests
		w.Header().Set("Access-Control-Max-Age", "3600")

		// Handle preflight OPTIONS requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// orderRouteHandler handles all routes starting with /orders/
func orderRouteHandler(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")

	// Check if the path has enough parts
	if len(pathParts) < 3 {
		http.Error(w, "Invalid URL path", http.StatusBadRequest)
		return
	}

	orderId := pathParts[2]

	// Handle /orders/{orderId}/status
	if len(pathParts) >= 4 && pathParts[3] == "status" {
		handleOrderStatus(w, r, orderId)
		return
	}

	// Handle /orders/{orderId} - for future use
	if len(pathParts) == 3 {
		handleSingleOrder(w, r, orderId)
		return
	}

	http.Error(w, "Invalid URL path", http.StatusBadRequest)
}

// handleSingleOrder handles GET and PUT requests for a single order
func handleSingleOrder(w http.ResponseWriter, r *http.Request, orderIdStr string) {
	switch r.Method {
	case http.MethodGet:
		// Get a single order
		getOrderById(w, r, orderIdStr)
	case http.MethodPut:
		// Update an order
		updateOrder(w, r, orderIdStr)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getOrderById retrieves a single order by ID
func getOrderById(w http.ResponseWriter, r *http.Request, orderIdStr string) {
	objectID, err := primitive.ObjectIDFromHex(orderIdStr)
	if err != nil {
		http.Error(w, "Invalid order ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var order Order
	err = orderColl.FindOne(ctx, bson.M{"_id": objectID}).Decode(&order)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			http.Error(w, "Order not found", http.StatusNotFound)
		} else {
			log.Printf("Error finding order: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

// updateOrder updates an existing order
func updateOrder(w http.ResponseWriter, r *http.Request, orderIdStr string) {
	objectID, err := primitive.ObjectIDFromHex(orderIdStr)
	if err != nil {
		http.Error(w, "Invalid order ID", http.StatusBadRequest)
		return
	}

	var updatedOrder Order
	if err := json.NewDecoder(r.Body).Decode(&updatedOrder); err != nil {
		http.Error(w, "Invalid JSON data", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Ensure ID is not changed
	updatedOrder.ID = objectID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Update the order
	_, err = orderColl.ReplaceOne(ctx, bson.M{"_id": objectID}, updatedOrder)
	if err != nil {
		log.Printf("Error updating order: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedOrder)
}

// handleOrderStatus handles status updates for an order
func handleOrderStatus(w http.ResponseWriter, r *http.Request, orderIdStr string) {
	switch r.Method {
	case http.MethodGet:
		getOrderStatus(w, r, orderIdStr)
	case http.MethodPatch:
		updateOrderStatus(w, r, orderIdStr)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getOrderStatus retrieves the status of an order
func getOrderStatus(w http.ResponseWriter, r *http.Request, orderIdStr string) {
	objectID, err := primitive.ObjectIDFromHex(orderIdStr)
	if err != nil {
		http.Error(w, "Invalid order ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var order Order
	err = orderColl.FindOne(ctx, bson.M{"_id": objectID}).Decode(&order)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			http.Error(w, "Order not found", http.StatusNotFound)
		} else {
			log.Printf("Error finding order: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	status := map[string]string{
		"orderID": orderIdStr,
		"status":  order.Status,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// updateOrderStatus updates the status of an order
func updateOrderStatus(w http.ResponseWriter, r *http.Request, orderIdStr string) {
	objectID, err := primitive.ObjectIDFromHex(orderIdStr)
	if err != nil {
		http.Error(w, "Invalid order ID", http.StatusBadRequest)
		return
	}

	var statusUpdate StatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&statusUpdate); err != nil {
		http.Error(w, "Invalid JSON data", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if statusUpdate.Status == "" {
		http.Error(w, "Status cannot be empty", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Update only the status field
	update := bson.M{
		"$set": bson.M{"status": statusUpdate.Status},
	}

	result, err := orderColl.UpdateOne(ctx, bson.M{"_id": objectID}, update)
	if err != nil {
		log.Printf("Error updating order status: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if result.MatchedCount == 0 {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	// Get the updated order
	var updatedOrder Order
	err = orderColl.FindOne(ctx, bson.M{"_id": objectID}).Decode(&updatedOrder)
	if err != nil {
		log.Printf("Error retrieving updated order: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedOrder)
}

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

	// Set default status
	if order.Status == "" {
		order.Status = "scheduled"
	}

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

func getOrdersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET is allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := orderColl.Find(ctx, bson.M{})
	if err != nil {
		log.Printf("Error finding orders: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	var orders []Order
	if err = cursor.All(ctx, &orders); err != nil {
		log.Printf("Error decoding orders: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
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

	if !isValidEmail(o.Email) {
		return errors.New("invalid email format")
	}

	return nil
}

// parseDeliveryDateTime combines deliveryDate and deliveryTime.
func parseDeliveryDateTime(dateStr, timeStr string) (time.Time, error) {
	layout := "2006-01-02 15:04"
	combined := fmt.Sprintf("%s %s", dateStr, timeStr)
	return time.Parse(layout, combined)
}

// isValidEmail provides a basic check for "@" presence.
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
